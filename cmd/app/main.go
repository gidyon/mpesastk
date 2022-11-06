package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"time"

	"github.com/gidyon/gomicro"
	"github.com/gidyon/gomicro/pkg/conn"
	grpcauth "github.com/gidyon/gomicro/pkg/grpc/auth"
	"github.com/gidyon/gomicro/pkg/grpc/zaplogger"
	"github.com/gidyon/gomicro/utils/errs"
	"github.com/gidyon/kongauth"
	stk_app_v1 "github.com/gidyon/mpesastk/internal/stk/v1"
	stk_v1 "github.com/gidyon/mpesastk/pkg/api/stk/v1"
	"github.com/go-redis/redis/v8"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	app_grpc_middleware "github.com/gidyon/gomicro/pkg/grpc"
)

var configFile = flag.String("config-file", ".env", "Configuration file")

func main() {
	flag.Parse()

	ctx := context.Background()

	// Config in .env
	viper.SetConfigFile(*configFile)

	// Read config from .env
	err := viper.ReadInConfig()
	errs.Panic(err)

	// Initialize logger
	errs.Panic(zaplogger.Init(viper.GetInt("logLevel"), ""))

	zaplogger.Log = zaplogger.Log.WithOptions(zap.WithCaller(true))

	// gRPC logger compatible
	appLogger := zaplogger.ZapGrpcLoggerV2(zaplogger.Log)

	// New service instance
	app, err := gomicro.NewService(&gomicro.Options{
		ServiceName:        viper.GetString("appName"),
		HttpPort:           viper.GetInt("httpPort"),
		GrpcPort:           viper.GetInt("grpcPort"),
		Logger:             appLogger,
		RuntimeMuxEndpoint: "/",
		ServerReadTimeout:  viper.GetDuration("serverReadTimeout"),
		ServerWriteTimeout: viper.GetDuration("serverWriteTimeout"),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		TLSEnabled:    viper.GetBool("tlsEnabled"),
		TlSCertFile:   viper.GetString("tlsCert"),
		TlSKeyFile:    viper.GetString("tlsKey"),
		TLSServerName: viper.GetString("tlsSubjectAltName"),
	})
	errs.Panic(err)

	// Open gorm connection
	sqlDB, err := conn.OpenGorm(&conn.DbOptions{
		Name:     viper.GetString("mysqlName"),
		Dialect:  viper.GetString("mysqlDialect"),
		Address:  viper.GetString("mysqlAddress"),
		User:     viper.GetString("mysqlUser"),
		Password: viper.GetString("mysqlPassword"),
		Schema:   viper.GetString("mysqlSchema"),
		ConnPool: &conn.DbPoolSettings{
			MaxIdleConns: viper.GetUint("mysqlMaxIdleConns"),
			MaxOpenConns: viper.GetUint("mysqlMaxOpenConns"),
			MaxLifetime:  viper.GetDuration("mysqlMaxLifetime"),
		},
	})
	errs.Panic(err)

	sqlDB = sqlDB.Debug()

	// Open redis connection
	redisDB := redis.NewClient(&redis.Options{
		Network:      viper.GetString("redisNetwork"),
		Addr:         viper.GetString("redisAddress"),
		Username:     viper.GetString("redisUsername"),
		Password:     viper.GetString("redisPassword"),
		DB:           viper.GetInt("redisDB"),
		MaxRetries:   viper.GetInt("redisMaxRetries"),
		ReadTimeout:  viper.GetDuration("redisReadTimeout"),
		WriteTimeout: viper.GetDuration("redisWriteTimeout"),
		MinIdleConns: viper.GetInt("redisMinIdleConns"),
		MaxConnAge:   viper.GetDuration("redisMaxConnAge"),
	})

	// Recovery middleware
	recoveryUIs, recoverySIs := app_grpc_middleware.AddRecovery()
	app.AddGRPCUnaryServerInterceptors(recoveryUIs...)
	app.AddGRPCStreamServerInterceptors(recoverySIs...)

	// Logging middleware
	logginUIs, loggingSIs := app_grpc_middleware.AddLogging(zaplogger.Log)
	app.AddGRPCUnaryServerInterceptors(logginUIs...)
	app.AddGRPCStreamServerInterceptors(loggingSIs...)

	jwtKey := viper.GetString("JWT_SIGNING_KEY")
	if jwtKey == "" {
		errs.Panic(errors.New("missing JWT key"))
	}

	jwtKeyByte := []byte(jwtKey)

	// Authentication API
	authAPI := grpcauth.NewAPI(jwtKeyByte, "ONFON MEDIA LIMITED", "apis")

	// Add admins
	authAPI.AddAdminGroups(viper.GetStringSlice("ADMIN_GROUPS")...)

	// Authentication middleware
	authUIs, authSIs := app_grpc_middleware.AddAuth(func(ctx context.Context) (context.Context, error) {
		// Call custom auth implementation
		return kongauth.Authenticator(ctx, &kongauth.AuthOptions{
			AuthAPI: authAPI,
			SqlDB:   sqlDB,
			RedisDB: redisDB,
			Logger:  appLogger,
		})
	})
	app.AddGRPCUnaryServerInterceptors(authUIs...)
	app.AddGRPCStreamServerInterceptors(authSIs...)

	// Servemux option for JSON Marshaling
	app.AddRuntimeMuxOptions(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			EmitUnpopulated: true,
		},
	}))

	// CORS settings
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Access-Control-Allow-Origin",
			"Authorization",
			"Cache-Control",
			"Content-Type",
			"DNT",
			"If-Modified-Since",
			"Keep-Alive",
			"Origin",
			"User-Agent",
			"X-Requested-With",
		},
		ExposedHeaders:       []string{"Authorization"},
		MaxAge:               1728,
		AllowCredentials:     true,
		OptionsPassthrough:   false,
		OptionsSuccessStatus: 0,
		Debug:                true,
	})

	// Middleware for CORS
	app.AddHTTPMiddlewares(func(h http.Handler) http.Handler {
		return c.Handler(h)
	})

	// Start service
	app.Start(ctx, func() error {

		var stkCallbackV1 = firstVal(viper.GetString("STK_RESULT_URL"))

		// STK V1
		stkV1, err := stk_app_v1.NewStkAPI(ctx, &stk_app_v1.Options{
			SQLDB:   sqlDB,
			RedisDB: redisDB,
			Logger:  appLogger,
			AuthAPI: authAPI,
			OptionSTK: &stk_app_v1.OptionSTK{
				AccessTokenURL:    viper.GetString("MPESA_ACCESS_TOKEN_URL"),
				ConsumerKey:       firstVal(viper.GetString("STK_CONSUMER_KEY"), viper.GetString("SAF_CONSUMER_KEY")),
				ConsumerSecret:    firstVal(viper.GetString("STK_CONSUMER_SECRET"), viper.GetString("SAF_CONSUMER_SECRET")),
				BusinessShortCode: viper.GetString("STK_BUSINESS_SHORT_CODE"),
				AccountReference:  viper.GetString("STK_MPESA_ACCOUNT_REFERENCE"),
				Timestamp:         viper.GetString("STK_MPESA_ACCESS_TIMESTAMP"),
				PassKey:           viper.GetString("STK_LNM_PASSKEY"),
				CallBackURL:       stkCallbackV1,
				PostURL:           viper.GetString("STK_MPESA_POST_URL"),
				QueryURL:          viper.GetString("STK_MPESA_QUERY_URL"),
			},
			HTTPClient:                http.DefaultClient,
			UpdateAccessTokenDuration: 0,
		})
		errs.Panic(err)

		stk_v1.RegisterStkPushV1Server(app.GRPCServer(), stkV1)
		errs.Panic(stk_v1.RegisterStkPushV1Handler(ctx, app.RuntimeMux(), app.ClientConn()))

		// Options for gateways
		opts := &Options{
			SQLDB:    sqlDB,
			RedisDB:  redisDB,
			Logger:   appLogger,
			AuthAPI:  authAPI,
			StkV1API: stkV1,
		}

		// MPESA STK Push gateway
		stkGateway, err := NewSTKGateway(ctx, opts)
		errs.Panic(err)

		// V1 endpoint
		app.AddEndpointFunc("/v1/mpesastkIncoming", stkGateway.ServeStkV1)
		appLogger.Infof("STK callback path: %v", stkCallbackV1)

		return nil
	})
}

func firstVal(vals ...string) string {
	for _, val := range vals {
		if val != "" {
			return val
		}
	}
	return ""
}
