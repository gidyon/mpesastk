package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	auth "github.com/gidyon/gomicro/pkg/grpc/auth"
	stk_app_v1 "github.com/gidyon/mpesastk/internal/stk/v1"
	stk_v1 "github.com/gidyon/mpesastk/pkg/api/stk/v1"
	"github.com/gidyon/mpesastk/pkg/payload"
	"github.com/gidyon/mpesastk/pkg/utils/httputils"
	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

type Options struct {
	SQLDB    *gorm.DB
	RedisDB  *redis.Client
	Logger   grpclog.LoggerV2
	AuthAPI  *auth.API
	StkV1API stk_v1.StkPushV1Server
}

func validateOptions(opt *Options) error {
	var err error
	switch {
	case opt == nil:
		err = errors.New("missing options")
	case opt.SQLDB == nil:
		err = errors.New("missing sqlDB")
	case opt.RedisDB == nil:
		err = errors.New("missing redisDB")
	case opt.Logger == nil:
		err = errors.New("missing logger")
	case opt.AuthAPI == nil:
		err = errors.New("missing auth API")
	case opt.StkV1API == nil:
		err = errors.New("missing stk v1 API")
	}
	return err
}

type stkGateway struct {
	ctxExt context.Context
	*Options
}

// NewSTKGateway creates a new mpesa stkGateway
func NewSTKGateway(ctx context.Context, opt *Options) (*stkGateway, error) {
	err := validateOptions(opt)
	if err != nil {
		return nil, err
	}

	gw := &stkGateway{
		Options: opt,
	}

	// Generate token
	token, err := gw.AuthAPI.GenToken(
		ctx, &auth.Payload{Group: auth.DefaultAdminGroup()}, time.Now().Add(10*365*24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth token: %v", err)
	}

	md := metadata.Pairs(auth.Header(), fmt.Sprintf("%s %s", auth.Scheme(), token))

	ctxExt := metadata.NewIncomingContext(ctx, md)

	// Authenticate the token
	gw.ctxExt, err = gw.AuthAPI.Authenticator(ctxExt)
	if err != nil {
		return nil, err
	}

	return gw, nil
}

func (gw *stkGateway) ServeStkV1(w http.ResponseWriter, r *http.Request) {
	code, err := gw.serveStkV1(w, r)
	if err != nil {
		gw.Logger.Errorf("Error serving incoming Stk V1 Transaction %v", err)
		http.Error(w, "request handler failed", code)
	}
}

func (gw *stkGateway) serveStkV1(w http.ResponseWriter, r *http.Request) (int, error) {

	httputils.DumpRequest(r, "Incoming Mpesa STK V2 Payload")

	if r.Method != http.MethodPost {
		return http.StatusBadRequest, fmt.Errorf("bad method; only POST allowed; received %v method", r.Method)
	}

	var (
		err        error
		stkPayload = &payload.STKPayload{}
		db         = &stk_app_v1.STKTransaction{}
		succeeded  = "YES"
		status     = stk_v1.StkStatus_STK_SUCCESS.String()
		initReq    = &stk_v1.InitiateSTKRequest{}
	)

	// Marshal incoming stk payload data
	{
		switch r.Header.Get("content-type") {
		case "application/json", "application/json;charset=UTF-8":
			err = json.NewDecoder(r.Body).Decode(stkPayload)
			if err != nil {
				return http.StatusBadRequest, fmt.Errorf("decoding json failed: %w", err)
			}
		default:
			return http.StatusBadRequest, fmt.Errorf("incorrect content type: %v", r.Header.Get("content-type"))
		}
	}

	// Validate incoming stk payload
	{
		switch {
		case stkPayload == nil:
			err = fmt.Errorf("nil stk transaction")
		case stkPayload.Body.STKCallback.CheckoutRequestID == "":
			err = fmt.Errorf("missing checkout id")
		case stkPayload.Body.STKCallback.MerchantRequestID == "":
			err = fmt.Errorf("missing merchant id")
		case stkPayload.Body.STKCallback.ResultDesc == "":
			err = fmt.Errorf("missing description")
		}
		if err != nil {
			return http.StatusBadRequest, err
		}
	}

	if stkPayload.Body.STKCallback.ResultCode != 0 {
		succeeded = "NO"
		status = stk_v1.StkStatus_STK_FAILED.String()
	}

	// Get the request that initiated this STK
	{
		ctx := r.Context()

		bs, err := gw.RedisDB.Get(ctx, stk_app_v1.GetMpesaRequestKey(stkPayload.Body.STKCallback.CheckoutRequestID)).Result()
		switch {
		case err == nil:
			err = proto.Unmarshal([]byte(bs), initReq)
			if err != nil {
				gw.Logger.Errorln("Failed to unmarshal initiate stk request: ", err)
			}
		}
	}

	err = gw.SQLDB.First(db, "checkout_request_id = ?", stkPayload.Body.STKCallback.CheckoutRequestID).Error
	switch {
	case err == nil:
		// Update STK transaction
		{
			err = gw.SQLDB.Model(db).Updates(map[string]interface{}{
				"result_code":        sql.NullString{String: fmt.Sprint(stkPayload.Body.STKCallback.ResultCode), Valid: fmt.Sprint(stkPayload.Body.STKCallback.ResultCode) != ""},
				"result_description": sql.NullString{String: stkPayload.Body.STKCallback.ResultDesc, Valid: stkPayload.Body.STKCallback.ResultDesc != ""},
				"mpesa_receipt_id":   sql.NullString{String: firstVal(stkPayload.Body.STKCallback.CallbackMetadata.MpesaReceiptNumber(), db.MpesaReceiptId.String), Valid: firstVal(stkPayload.Body.STKCallback.CallbackMetadata.MpesaReceiptNumber(), db.MpesaReceiptId.String) != ""},
				"transaction_time":   sql.NullTime{Valid: true, Time: stkPayload.Body.STKCallback.CallbackMetadata.GetTransTime()},
				"stk_status":         sql.NullString{String: status, Valid: true},
				"succeeded":          succeeded,
			}).Error
			if err != nil {
				return http.StatusInternalServerError, fmt.Errorf("failed to update stk: %v", err)
			}
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Create STK transaction
		{
			success := "NO"
			if stkPayload.Body.STKCallback.ResultCode != 0 {
				success = "YES"
			}

			db = &stk_app_v1.STKTransaction{
				ID:                         0,
				InitiatorID:                initReq.GetInitiatorId(),
				InitiatorCustomerReference: initReq.GetInitiatorCustomerReference(),
				InitiatorCustomerNames:     initReq.GetInitiatorCustomerNames(),
				PhoneNumber:                stkPayload.Body.STKCallback.CallbackMetadata.PhoneNumber(),
				Amount:                     fmt.Sprint(stkPayload.Body.STKCallback.CallbackMetadata.GetAmount()),
				ShortCode:                  initReq.PublishMessage.Payload["short_code"],
				AccountReference:           initReq.GetAccountReference(),
				TransactionDesc:            sql.NullString{String: initReq.GetTransactionDesc(), Valid: initReq.GetTransactionDesc() != ""},
				MerchantRequestID:          sql.NullString{String: stkPayload.Body.STKCallback.MerchantRequestID, Valid: stkPayload.Body.STKCallback.MerchantRequestID != ""},
				CheckoutRequestID:          sql.NullString{String: stkPayload.Body.STKCallback.MerchantRequestID, Valid: stkPayload.Body.STKCallback.MerchantRequestID != ""},
				ResultCode:                 sql.NullString{String: fmt.Sprint(stkPayload.Body.STKCallback.ResultCode), Valid: fmt.Sprint(stkPayload.Body.STKCallback.ResultCode) != ""},
				ResultDescription:          sql.NullString{String: stkPayload.Body.STKCallback.ResultDesc, Valid: stkPayload.Body.STKCallback.ResultDesc != ""},
				MpesaReceiptId:             sql.NullString{String: stkPayload.Body.STKCallback.CallbackMetadata.MpesaReceiptNumber(), Valid: stkPayload.Body.STKCallback.CallbackMetadata.MpesaReceiptNumber() != ""},
				StkStatus:                  sql.NullString{String: status, Valid: status != ""},
				Succeeded:                  success,
				Processed:                  "NO",
				TransactionTime:            sql.NullTime{Valid: true, Time: stkPayload.Body.STKCallback.CallbackMetadata.GetTransTime()},
				CreatedAt:                  time.Time{},
			}
			err = gw.SQLDB.Create(db).Error
			if err != nil {
				return http.StatusInternalServerError, fmt.Errorf("failed to create stk transaction: %v", err)
			}
		}
	default:
		gw.Logger.Errorln(err)
		return http.StatusInternalServerError, errors.New("failed to get stk transaction")
	}

	pb, err := stk_app_v1.ToProto(db)
	if err != nil {
		gw.Logger.Errorln(err)
		return http.StatusInternalServerError, errors.New("failed to get stk proto")
	}

	if initReq.GetPublish() {
		publish := func() {
			_, err = gw.StkV1API.PublishStkTransaction(gw.ctxExt, &stk_v1.PublishStkTransactionRequest{
				PublishMessage: &stk_v1.PublishMessage{
					InitiatorId:     initReq.InitiatorId,
					TransactionId:   pb.TransactionId,
					MpesaReceiptId:  pb.MpesaReceiptId,
					PhoneNumber:     pb.PhoneNumber,
					PublishInfo:     initReq.PublishMessage,
					TransactionInfo: pb,
				},
			})
			if err != nil {
				gw.Logger.Warningf("failed to publish message: %v", err)
			} else {
				gw.Logger.Infoln("STK has been published on channel ", initReq.GetPublishMessage().GetChannelName())
			}
		}
		if initReq.GetPublishMessage().GetOnlyOnSuccess() {
			if pb.Succeeded {
				publish()
			}
		} else {
			publish()
		}
	}

	_, err = w.Write([]byte("mpesa stk processed"))
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}
