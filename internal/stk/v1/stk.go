package stk

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	auth "github.com/gidyon/gomicro/pkg/grpc/auth"
	"github.com/gidyon/gomicro/utils/errs"
	"github.com/gidyon/mpesapayments/pkg/utils/formatutil"
	"github.com/gidyon/mpesapayments/pkg/utils/httputils"
	stk "github.com/gidyon/mpesastk/pkg/api/stk/v1"
	redis "github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// HTTPClient makes mocking test easier
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type stkAPIServer struct {
	stk.UnsafeStkPushV1Server
	*Options
}

// Options contain parameters passed for creating stk service
type Options struct {
	SQLDB                     *gorm.DB
	RedisDB                   *redis.Client
	Logger                    grpclog.LoggerV2
	AuthAPI                   *auth.API
	OptionSTK                 *OptionSTK
	HTTPClient                HTTPClient
	UpdateAccessTokenDuration time.Duration
	AllowQueryStatus          bool
	SystemIdPrefix            string
	PublishProcessChannel     string
}

// ValidateOptions validates options required by stk service
func ValidateOptions(opt *Options) error {
	var err error
	switch {
	case opt == nil:
		err = errs.MissingField("options")
	case opt.SQLDB == nil:
		err = errs.MissingField("sql db")
	case opt.RedisDB == nil:
		err = errs.MissingField("redis db")
	case opt.Logger == nil:
		err = errs.MissingField("logger")
	case opt.AuthAPI == nil:
		err = errs.MissingField("auth API")
	case opt.HTTPClient == nil:
		err = errs.MissingField("http client")
	case opt.OptionSTK == nil:
		err = errs.MissingField("stk options")
	}
	return err
}

// OptionSTK contains options for sending push stk
type OptionSTK struct {
	AccessTokenURL    string
	PassKey           string
	ConsumerKey       string
	ConsumerSecret    string
	BusinessShortCode string
	AccountReference  string
	Timestamp         string
	CallBackURL       string
	PostURL           string
	QueryURL          string
	password          string
	accessToken       string
	basicToken        string
}

// ValidateOptionSTK validates stk options
func ValidateOptionSTK(opt *OptionSTK) error {
	var err error
	switch {
	case opt == nil:
		err = errs.MissingField("stk options")
	case opt.AccessTokenURL == "":
		err = errs.MissingField("access token url")
	case opt.ConsumerKey == "":
		err = errs.MissingField("consumer key")
	case opt.ConsumerSecret == "":
		err = errs.MissingField("consumer secret")
	case opt.BusinessShortCode == "":
		err = errs.MissingField("business short code")
	case opt.AccountReference == "":
		err = errs.MissingField("account reference")
	case opt.Timestamp == "":
		err = errs.MissingField("timestamp")
	case opt.PassKey == "":
		err = errs.MissingField("pass key")
	case opt.CallBackURL == "":
		err = errs.MissingField("callback url")
	case opt.PostURL == "":
		err = errs.MissingField("post url")
	case opt.QueryURL == "":
	case opt.accessToken == "":
	case opt.basicToken == "":
	}
	return err
}

// NewStkAPI creates a singleton instance of mpesa stk API
func NewStkAPI(ctx context.Context, opt *Options) (_ stk.StkPushV1Server, err error) {

	defer func() {
		if err != nil {
			err = errs.WrapErrorWithMsgFunc("Failed to start STK API service")(err)
		}
	}()

	// Validation
	switch {
	case ctx == nil:
		return nil, errs.MissingField("context")
	default:
		err = ValidateOptions(opt)
		if err != nil {
			return nil, err
		}
		err = ValidateOptionSTK(opt.OptionSTK)
		if err != nil {
			return nil, err
		}
	}

	// Update Basic Token
	opt.OptionSTK.basicToken = base64.StdEncoding.EncodeToString([]byte(
		opt.OptionSTK.ConsumerKey + ":" + opt.OptionSTK.ConsumerSecret,
	))

	// Update Password
	opt.OptionSTK.password = base64.StdEncoding.EncodeToString([]byte(
		opt.OptionSTK.BusinessShortCode + opt.OptionSTK.PassKey + opt.OptionSTK.Timestamp,
	))

	// API server
	stkAPI := &stkAPIServer{
		Options: opt,
	}

	// Auto migration
	if !stkAPI.SQLDB.Migrator().HasTable(&STKTransaction{}) {
		err = stkAPI.SQLDB.Migrator().AutoMigrate(&STKTransaction{})
		if err != nil {
			return nil, err
		}
	}

	dur := time.Minute * 15
	if opt.UpdateAccessTokenDuration > 0 {
		dur = opt.UpdateAccessTokenDuration
	}

	// Worker for updating access token
	go stkAPI.updateAccessTokenWorker(ctx, dur)

	// Worker for updating STK results
	if opt.AllowQueryStatus {
		go stkAPI.updateSTKResultsWorker(ctx, time.Minute*5)
	}

	if opt.PublishProcessChannel != "" {
		go stkAPI.processWorker(ctx)
	}

	return stkAPI, nil
}

// GetMpesaSTKPushKey retrives key storing initiator key
func GetMpesaSTKPushKey(msisdn string) string {
	return fmt.Sprintf("stkpush:%s", msisdn)
}

// GetMpesaRequestKey is key that initiates data
func GetMpesaRequestKey(requestId string) string {
	return fmt.Sprintf("stk:%s", requestId)
}

func firstVal(A ...string) string {
	for _, s := range A {
		if s != "" {
			return s
		}
	}
	return ""
}

func (stkAPI *stkAPIServer) InitiateSTK(
	ctx context.Context, req *stk.InitiateSTKRequest,
) (*stk.InitiateSTKResponse, error) {
	// Validation
	switch {
	case req == nil:
		return nil, errs.MissingField("request")
	case req.InitiatorId == "":
		return nil, errs.MissingField("inititator id")
	case req.Phone == "":
		return nil, errs.MissingField("phone")
	case req.AccountReference == "":
		return nil, errs.MissingField("account reference")
	case req.Amount <= 0:
		return nil, errs.MissingField("amount")
	case req.Publish && req.GetPublishMessage().GetChannelName() == "":
		return nil, errs.MissingField("publisch channel")
	}

	var (
		phoneNumber = formatutil.FormatPhoneKE(req.Phone)
		shortCode   = firstVal(req.ShortCode, stkAPI.OptionSTK.BusinessShortCode)
		accountRef  = firstVal(req.AccountReference, stkAPI.OptionSTK.AccountReference)
		pb          = &STKRequestBody{
			BusinessShortCode: shortCode,
			Password:          stkAPI.OptionSTK.password,
			Timestamp:         stkAPI.OptionSTK.Timestamp,
			TransactionType:   "CustomerPayBillOnline",
			Amount:            fmt.Sprint(req.Amount),
			PartyA:            phoneNumber,
			PartyB:            shortCode,
			PhoneNumber:       phoneNumber,
			CallBackURL:       stkAPI.OptionSTK.CallBackURL,
			AccountReference:  accountRef,
			TransactionDesc:   firstVal(req.TransactionDesc, "NA"),
		}
		err error
	)

	if req.PublishMessage == nil {
		req.PublishMessage = &stk.PublishInfo{
			Payload: map[string]string{},
		}
	}

	if req.PublishMessage.Payload == nil {
		req.PublishMessage.Payload = map[string]string{}
	}

	req.PublishMessage.Payload["short_code"] = shortCode

	// Marshal request
	bs, err := json.Marshal(pb)
	if err != nil {
		return nil, errs.FromJSONMarshal(err, "pb")
	}

	// Create Mpesa STK request
	reqHtpp, err := http.NewRequest(http.MethodPost, stkAPI.OptionSTK.PostURL, bytes.NewReader(bs))
	if err != nil {
		return nil, errs.WrapMessage(codes.Internal, "failed to create post stk request")
	}

	// Update headers
	reqHtpp.Header.Set("Authorization", fmt.Sprintf("Bearer %s", stkAPI.OptionSTK.accessToken))
	reqHtpp.Header.Set("Content-Type", "application/json")

	httputils.DumpRequest(reqHtpp, "INITIATE STK REQUEST")

	// STK model
	db := &STKTransaction{
		ID:                         0,
		InitiatorID:                req.InitiatorId,
		InitiatorCustomerReference: req.InitiatorCustomerReference,
		InitiatorCustomerNames:     req.InitiatorCustomerNames,
		PhoneNumber:                phoneNumber,
		Amount:                     fmt.Sprint(req.Amount),
		ShortCode:                  shortCode,
		AccountReference:           accountRef,
		TransactionDesc:            sql.NullString{String: req.TransactionDesc, Valid: true},
		StkStatus:                  sql.NullString{String: stk.StkStatus_STK_REQUEST_SUBMITED.String(), Valid: true},
		TransactionTime:            sql.NullTime{Valid: true, Time: time.Now().UTC()},
		CreatedAt:                  time.Time{},
	}

	// Save the request to database
	err = stkAPI.SQLDB.Create(db).Error
	if err != nil {
		stkAPI.Logger.Errorln(err)
		return nil, errs.WrapMessage(codes.Internal, "failed to save stk")
	}

	go func() {
		err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := stkAPI.HTTPClient.Do(reqHtpp)
			if err != nil {
				return fmt.Errorf("failed to post stk request to mpesa API: %v", err)
			}

			httputils.DumpResponse(res, "INITIATE STK RESPONSE")

			resData := make(map[string]interface{})

			err = json.NewDecoder(res.Body).Decode(&resData)
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to decode mpesa response: %v", err)
			}

			errMsg, ok := resData["errorMessage"]
			if ok {
				return fmt.Errorf("error happened while sending stk push: %v", errMsg)
			}

			switch strings.ToLower(res.Header.Get("content-type")) {
			case "application/json", "application/json;charset=utf-8":
				// The CheckoutRequestID must exist
				_, ok := resData["CheckoutRequestID"].(string)
				if !ok {
					return errors.New("stk request failed: missing CheckoutRequestID")
				}

				// Update STK
				err = stkAPI.SQLDB.Model(db).Updates(&STKTransaction{
					MerchantRequestID:          sql.NullString{String: fmt.Sprint(resData["MerchantRequestID"]), Valid: fmt.Sprint(resData["MerchantRequestID"]) != ""},
					CheckoutRequestID:          sql.NullString{String: fmt.Sprint(resData["CheckoutRequestID"]), Valid: fmt.Sprint(resData["CheckoutRequestID"]) != ""},
					StkResponseDescription:     sql.NullString{String: fmt.Sprint(resData["ResponseDescription"]), Valid: fmt.Sprint(resData["ResponseDescription"]) != ""},
					StkResponseCustomerMessage: sql.NullString{String: fmt.Sprint(resData["CustomerMessage"]), Valid: fmt.Sprint(resData["CustomerMessage"]) != ""},
					StkResponseCode:            sql.NullString{String: fmt.Sprint(resData["ResponseCode"]), Valid: fmt.Sprint(resData["ResponseCode"]) != ""},
					StkStatus:                  sql.NullString{String: stk.StkStatus_STK_REQUEST_SUCCESS.String(), Valid: true},
					Succeeded:                  "NO",
					Processed:                  "NO",
					TransactionTime:            sql.NullTime{Valid: true, Time: time.Now().UTC()},
					CreatedAt:                  time.Time{},
				}).Error
				if err != nil {
					stkAPI.Logger.Errorln(err)
					return errors.New("failed to update stk payload")
				}

				// Marshal request
				bs, err := proto.Marshal(req)
				if err != nil {
					return fmt.Errorf("failed to proto marshal initiate stk request: %v", err)
				}

				requestId := GetMpesaRequestKey(fmt.Sprint(resData["CheckoutRequestID"]))

				// Save request to cache
				err = stkAPI.RedisDB.Set(ctx, requestId, bs, time.Minute*15).Err()
				if err != nil {
					return fmt.Errorf("failed to set initiate stk request to cache: %v", err)
				}
			default:
				return errors.New("incorrect response while initiating STK")
			}

			return nil
		}()
		if err != nil {
			// Update status to failed
			err = stkAPI.SQLDB.Model(db).Updates(&STKTransaction{
				StkResponseDescription: sql.NullString{String: err.Error(), Valid: true},
				StkStatus:              sql.NullString{String: stk.StkStatus_STK_REQUEST_FAILED.String(), Valid: true},
				Succeeded:              "NO",
				Processed:              "NO",
				TransactionTime:        sql.NullTime{Valid: true, Time: time.Now().UTC()},
				CreatedAt:              time.Time{},
			}).Error
			if err != nil {
				stkAPI.Logger.Errorln(err)
			}
		}
	}()

	return &stk.InitiateSTKResponse{
		Progress: true,
		Message:  "Processing. Stk popup will come shortly",
	}, nil
}

func (stkAPI *stkAPIServer) GetStkTransaction(
	ctx context.Context, req *stk.GetStkTransactionRequest,
) (*stk.StkTransaction, error) {
	var err error

	// Validation
	var key int
	switch {
	case req == nil:
		return nil, errs.MissingField("request")
	case req.TransactionId == 0 && req.MpesaReceiptId == "":
		return nil, errs.MissingField("transaction/mpesa id")
	}

	db := &STKTransaction{}

	if req.TransactionId != 0 {
		err = stkAPI.SQLDB.First(db, "id=?", key).Error
	} else if req.MpesaReceiptId != "" {
		err = stkAPI.SQLDB.First(db, "mpesa_receipt_id=?", req.MpesaReceiptId).Error
	}
	switch {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, status.Errorf(codes.NotFound, "stk transaction with id %d does not exist", req.TransactionId)
	default:
		stkAPI.Logger.Errorln(err)
		return nil, errs.WrapMessage(codes.Internal, "failed to get stk transaction")
	}

	return ToProto(db)
}

const defaultPageSize = 100

func userAllowedPhonesSet(userkey string) string {
	return fmt.Sprintf("stk:user:%s:allowedphones", userkey)
}

func (stkAPI *stkAPIServer) ListStkTransactions(
	ctx context.Context, req *stk.ListStkTransactionsRequest,
) (*stk.ListStkTransactionsResponse, error) {
	// Authorization
	actor, err := stkAPI.AuthAPI.GetPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Validation
	switch {
	case req == nil:
		return nil, errs.MissingField("list request")
	case req.PageSize < 0:
		return nil, errs.IncorrectVal("page size")
	}

	// Read from redis list of phone numbers
	var allowedPhones []string
	allowedPhones, err = stkAPI.RedisDB.SMembers(
		ctx, userAllowedPhonesSet(actor.ID),
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		stkAPI.Logger.Errorln(err)
		return nil, errs.WrapMessage(codes.Internal, "request failed")
	}

	pageSize := req.GetPageSize()
	switch {
	case pageSize <= 0:
		pageSize = defaultPageSize
	case pageSize > defaultPageSize:
		if !stkAPI.AuthAPI.IsAdmin(actor.Group) {
			pageSize = defaultPageSize
		}
	}

	var key string

	pageToken := req.GetPageToken()
	if pageToken != "" {
		bs, err := base64.StdEncoding.DecodeString(req.GetPageToken())
		if err != nil {
			return nil, errs.WrapErrorWithCodeAndMsg(codes.InvalidArgument, err, "failed to parse page token")
		}
		key = string(bs)
	}

	dbs := make([]*STKTransaction, 0, pageSize+1)

	db := stkAPI.SQLDB.Model(&STKTransaction{}).Limit(int(pageSize) + 1)
	if key != "" {
		switch req.GetFilter().GetOrderField() {
		case stk.StkOrderField_CREATE_TIMESTAMP:
			db = db.Where("id<?", key).Order("id DESC")
		case stk.StkOrderField_TRANSACTION_TIMESTAMP:
			db = db.Where("transaction_time<?", key).Order("transaction_time DESC")
		}
	} else {
		switch req.GetFilter().GetOrderField() {
		case stk.StkOrderField_CREATE_TIMESTAMP:
			db = db.Order("id DESC")
		case stk.StkOrderField_TRANSACTION_TIMESTAMP:
			db = db.Order("transaction_time DESC")
		}
	}

	if len(allowedPhones) > 0 {
		db = db.Where("phone_number IN(?)", allowedPhones)
	}

	// Apply filters
	if req.Filter != nil {
		startTimestamp := req.Filter.GetStartTimestamp()
		endTimestamp := req.Filter.GetEndTimestamp()

		// Timestamp filter
		if endTimestamp > startTimestamp {
			switch req.GetFilter().GetOrderField() {
			case stk.StkOrderField_CREATE_TIMESTAMP:
				db = db.Where("created_at BETWEEN ? AND ?", time.Unix(startTimestamp, 0), time.Unix(endTimestamp, 0))
			case stk.StkOrderField_TRANSACTION_TIMESTAMP:
				db = db.Where("transaction_time BETWEEN ? AND ?", time.Unix(startTimestamp, 0), time.Unix(endTimestamp, 0))
			}
		} else if req.Filter.TxDate != "" {
			// Date filter
			t, err := getTime(req.Filter.TxDate)
			if err != nil {
				return nil, err
			}
			switch req.GetFilter().GetOrderField() {
			case stk.StkOrderField_CREATE_TIMESTAMP:
				db = db.Where("created_at BETWEEN ? AND ?", t, t.Add(time.Hour*24))
			case stk.StkOrderField_TRANSACTION_TIMESTAMP:
				db = db.Where("transaction_time BETWEEN ? AND ?", t, t.Add(time.Hour*24))
			}
		}

		if len(req.Filter.Msisdns) > 0 {
			db = db.Where("phone_number IN(?)", req.Filter.Msisdns)
		}

		if len(req.Filter.MpesaReceipts) > 0 {
			db = db.Where("mpesa_receipt_id IN(?)", req.Filter.MpesaReceipts)
		}

		if len(req.Filter.InitiatorCustomerReferences) > 0 {
			db = db.Where("initiator_customer_reference IN(?)", req.Filter.InitiatorCustomerReferences)
		}

		if len(req.Filter.InitiatorTransactionReferences) > 0 {
			db = db.Where("initiator_transaction_reference IN(?)", req.Filter.InitiatorTransactionReferences)
		}

		if len(req.Filter.ShortCodes) > 0 {
			db = db.Where("short_code IN(?)", req.Filter.ShortCodes)
		}

		if len(req.Filter.StkStatuses) > 0 {
			ss := make([]string, 0, len(req.Filter.StkStatuses))
			for _, s := range req.Filter.StkStatuses {
				ss = append(ss, s.String())
			}
			db = db.Where("stk_status IN(?)", ss)
		}

		switch req.Filter.ProcessState {
		case stk.StkProcessedState_STK_PROCESS_STATE_UNSPECIFIED:
		case stk.StkProcessedState_STK_NOT_PROCESSED:
			db = db.Where("processed=?", "NO")
		case stk.StkProcessedState_STK_PROCESSED:
			db = db.Where("processed=?", "YES")
		}
	}

	var collectionCount int64

	if pageToken == "" && req.View == stk.ListStkTransactionsView_BASIC_VIEW {
		err = db.Count(&collectionCount).Error
		if err != nil {
			stkAPI.Logger.Errorln(err)
			return nil, errs.WrapMessage(codes.Internal, "request failed")
		}
	}

	err = db.Find(&dbs).Error
	switch {
	case err == nil:
	default:
		stkAPI.Logger.Errorln(err)
		return nil, errs.WrapMessage(codes.Internal, "request failed")
	}

	pbs := make([]*stk.StkTransaction, 0, len(dbs))

	for i, db := range dbs {
		pb, err := ToProto(db)
		if err != nil {
			return nil, err
		}

		if i == int(pageSize) {
			break
		}

		pbs = append(pbs, pb)

		switch req.GetFilter().GetOrderField() {
		case stk.StkOrderField_CREATE_TIMESTAMP:
			key = fmt.Sprint(db.ID)
		case stk.StkOrderField_TRANSACTION_TIMESTAMP:
			key = db.TransactionTime.Time.UTC().Format(time.RFC3339)
		}
	}

	var token string
	if len(dbs) > int(pageSize) {
		// Next page token
		token = base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(key)))
	}

	return &stk.ListStkTransactionsResponse{
		NextPageToken:   token,
		StkTransactions: pbs,
		CollectionCount: collectionCount,
	}, nil
}

func getTime(dateStr string) (time.Time, error) {
	// 2020y 08m 16d 20h 41m 16s
	// "2006-01-02T15:04:05Z07:00"

	timeRFC3339Str := fmt.Sprintf("%sT00:00:00Z", dateStr)

	t, err := time.Parse(time.RFC3339, timeRFC3339Str)
	if err != nil {
		return time.Time{}, errs.WrapErrorWithCodeAndMsg(codes.InvalidArgument, err, "failed to parse date to time")
	}

	return t, nil
}

func (stkAPI *stkAPIServer) ProcessStkTransaction(
	ctx context.Context, req *stk.ProcessStkTransactionRequest,
) (*emptypb.Empty, error) {
	// Authorization
	_, err := stkAPI.AuthAPI.AuthorizeGroups(ctx, stkAPI.AuthAPI.AdminGroups()...)
	if err != nil {
		return nil, err
	}

	// Validation
	var key int
	switch {
	case req == nil:
		return nil, errs.MissingField("process request")
	case req.TransactionId == 0 && req.MpesaReceiptId == "":
		return nil, errs.MissingField("transaction/mpesa id")
	}

	processed := "NO"
	if req.Processed {
		processed = "YES"
	}

	if req.TransactionId != 0 {
		err = stkAPI.SQLDB.Model(&STKTransaction{}).Unscoped().Where("id=?", key).
			Update("processed", processed).Error
	} else {
		err = stkAPI.SQLDB.Model(&STKTransaction{}).Unscoped().Where("mpesa_receipt_id=?", req.MpesaReceiptId).
			Update("processed", processed).Error
	}
	switch {
	case err == nil:
	default:
		stkAPI.Logger.Errorln(err)
		return nil, errs.WrapMessage(codes.Internal, "failed to process stk transaction")
	}

	return &emptypb.Empty{}, nil
}

func (stkAPI *stkAPIServer) PublishStkTransaction(
	ctx context.Context, req *stk.PublishStkTransactionRequest,
) (*emptypb.Empty, error) {
	// Authorization
	_, err := stkAPI.AuthAPI.AuthorizeGroups(ctx, stkAPI.AuthAPI.AdminGroups()...)
	if err != nil {
		return nil, err
	}

	// Validation
	switch {
	case req == nil:
		return nil, errs.MissingField("publish request")
	case req.PublishMessage == nil:
		return nil, errs.MissingField("publish message")
	}

	pb := req.GetPublishMessage().GetTransactionInfo()

	// Marshal data
	bs, err := proto.Marshal(req.PublishMessage)
	if err != nil {
		return nil, errs.FromProtoMarshal(err, "publish message")
	}

	channel := req.GetPublishMessage().GetPublishInfo().GetChannelName()
	if channel == "" {
		return &emptypb.Empty{}, nil
	}

	// Publish based on state
	switch req.ProcessedState {
	case stk.StkProcessedState_STK_PROCESS_STATE_UNSPECIFIED:
		err = stkAPI.RedisDB.Publish(ctx, channel, bs).Err()
		if err != nil {
			return nil, errs.WrapMessagef(codes.Internal, "publish failed: %v", err)
		}
	case stk.StkProcessedState_STK_PROCESSED:
		// Publish only if the processed state is true
		if pb.GetProcessed() {
			err = stkAPI.RedisDB.Publish(ctx, channel, bs).Err()
			if err != nil {
				return nil, errs.WrapMessagef(codes.Internal, "publish failed: %v", err)
			}
		}
	case stk.StkProcessedState_STK_NOT_PROCESSED:
		// Publish only if the processed state is false
		if !pb.GetProcessed() {
			err = stkAPI.RedisDB.Publish(ctx, channel, bs).Err()
			if err != nil {
				return nil, errs.WrapMessagef(codes.Internal, "publish failed: %v", err)
			}
		}
	}

	return &emptypb.Empty{}, nil
}
