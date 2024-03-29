package stk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	auth "github.com/gidyon/gomicro/pkg/grpc/auth"
	"github.com/gidyon/mpesapayments/pkg/payload"
	"github.com/gidyon/mpesapayments/pkg/utils/httputils"
	stk "github.com/gidyon/mpesastk/pkg/api/stk/v1"
	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/metadata"
)

func (stkAPI *stkAPIServer) updateAccessTokenWorker(ctx context.Context, dur time.Duration) {
	var (
		err      error
		xdur     = dur
		expo     = time.Second * 10
		ticker   = time.NewTicker(xdur)
		callback = func() {
			err = stkAPI.updateAccessToken()
			if err != nil {
				stkAPI.Logger.Errorf("failed to update access token: %v", err)
				ticker.Reset(expo + xdur)
				expo = expo * 2
			} else {
				stkAPI.Logger.Infoln("access token updated")
				ticker.Reset(dur)
			}
		}
	)

	callback()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			callback()
		}
	}
}

func (stkAPI *stkAPIServer) updateAccessToken() error {
	req, err := http.NewRequest(http.MethodGet, stkAPI.OptionSTK.AccessTokenURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", stkAPI.OptionSTK.basicToken))

	httputils.DumpRequest(req, "STK ACCESS TOKEN REQUEST")

	res, err := stkAPI.HTTPClient.Do(req)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("request failed: %v", err)
	}

	httputils.DumpResponse(res, "STK ACCESS TOKEN RESPONSE")

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status code OK got: %v", res.Status)
	}

	resTo := make(map[string]interface{})
	err = json.NewDecoder(res.Body).Decode(&resTo)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to json decode response: %v", err)
	}

	stkAPI.OptionSTK.accessToken = fmt.Sprint(resTo["access_token"])

	return nil
}

func (stkAPI *stkAPIServer) updateSTKResultsWorker(ctx context.Context, dur time.Duration) {
	time.Sleep(time.Second * 20)
	for {
		count, err := stkAPI.updateSTKResults(ctx)
		if err != nil {
			stkAPI.Logger.Errorf("Failed to update STK Results: %v", err)
		} else {
			stkAPI.Logger.Infof("%d STK Results updated", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(dur):
		}
	}
}

func (stkAPI *stkAPIServer) updateSTKResults(ctx context.Context) (int, error) {
	if stkAPI.OptionSTK.accessToken == "" {
		return 0, errors.New("missing access token")
	}
	var (
		sem   = make(chan struct{}, 5)
		dbs   = make([]*STKTransaction, 0)
		mu    = &sync.Mutex{}
		res   = 0
		ID    = 0
		limit = 1000
		next  = true
		err   error
	)

	for next {
		err = stkAPI.SQLDB.Order("id desc").Limit(limit+1).Model(&STKTransaction{}).
			Find(&dbs, "stk_status = ? AND id > ? AND created_at < ?", stk.StkStatus_STK_REQUEST_SUBMITED.String(), ID, time.Now().Add(-time.Minute*10)).Error
		if err != nil {
			return 0, err
		}

		if len(dbs) <= limit {
			next = false
		}

		wg := &sync.WaitGroup{}

		for _, db := range dbs {
			ID = int(db.ID)
			wg.Add(1)

			go func(db *STKTransaction) {
				sem <- struct{}{}

				defer func() {
					wg.Done()
					<-sem
				}()

				err := stkAPI.updateSTKResult(ctx, db)
				if err != nil {
					stkAPI.Logger.Errorln("Failed to Update STK Result: ", err)
				} else {
					mu.Lock()
					res++
					mu.Unlock()
				}
			}(db)
		}

		wg.Wait()
	}

	return res, nil
}

func (stkAPI *stkAPIServer) updateSTKResult(_ context.Context, db *STKTransaction) error {
	req := payload.QueryStkRequest{
		BusinessShortCode: db.ShortCode,
		Password:          stkAPI.OptionSTK.password,
		Timestamp:         stkAPI.OptionSTK.Timestamp,
		CheckoutRequestID: db.CheckoutRequestID.String,
	}

	bs, err := json.Marshal(req)
	if err != nil {
		return err
	}

	reqHtpp, err := http.NewRequest(http.MethodPost, stkAPI.OptionSTK.QueryURL, bytes.NewReader(bs))
	if err != nil {
		return err
	}

	reqHtpp.Header.Set("Authorization", fmt.Sprintf("Bearer %s", stkAPI.OptionSTK.accessToken))
	reqHtpp.Header.Set("Content-Type", "application/json")

	httputils.DumpRequest(reqHtpp, "QUERY STK STATUS REQUEST")

	res, err := stkAPI.HTTPClient.Do(reqHtpp)
	if err != nil {
		return fmt.Errorf("failed to post stk query API: %v", err)
	}

	httputils.DumpResponse(res, "QUERY STK RESULT RESPONSE")

	resData := &payload.QueryStkResponse{}

	err = json.NewDecoder(res.Body).Decode(&resData)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to decode mpesa response: %v", err)
	}

	if resData.MerchantRequestID == "" || resData.CheckoutRequestID == "" || resData.ResultCode == "" {
		return errors.New("gotten error while posting to query stk API")
	}

	succeeded := resData.ResultCode == "0" && strings.Contains(strings.ToLower(resData.ResultDesc), "successfully")
	status := stk.StkStatus_STK_RESULT_SUCCESS.String()
	if !succeeded {
		succeeded = false
		status = stk.StkStatus_STK_RESULT_FAILED.String()
	}

	systemId := fmt.Sprintf("%s_%d_%s", firstVal(stkAPI.SystemIdPrefix, "ONFON"), time.Now().UnixNano(), db.MerchantRequestID.String)

	switch strings.ToLower(res.Header.Get("content-type")) {
	case "application/json", "application/json;charset=utf-8":
		// Update the STK results
		err = stkAPI.SQLDB.Model(db).Updates(map[string]interface{}{
			"stk_response_description": resData.ResponseDescription,
			"stk_response_code":        resData.ResponseCode,
			"result_description":       resData.ResultDesc,
			"result_code":              resData.ResultCode,
			"mpesa_receipt_id":         systemId,
			"stk_status":               status,
			"succeeded":                succeeded,
		}).Error
		if err != nil {
			stkAPI.Logger.Errorln("failed to updated stk transaction: ", err)
		}
	default:
		stkAPI.Logger.Errorln("incorrect response while querying stk API")
	}

	return nil
}

func (stkAPI *stkAPIServer) processWorker(ctx context.Context) {

	stkAPI.Logger.Infof("Listening for process requests on channel: %v", stkAPI.PublishProcessChannel)

	ch := stkAPI.RedisDB.Subscribe(ctx, stkAPI.PublishProcessChannel).Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			go func(msg *redis.Message) {
				stkAPI.Logger.Infoln("Received process request")

				datas := msg.PayloadSlice

				if len(datas) != 2 {
					datas = strings.Split(msg.Payload, "/")
				}
				if len(datas) != 2 {
					stkAPI.Logger.Infoln("Incorrect payload")
					return
				}

				token, err := stkAPI.AuthAPI.GenToken(
					ctx,
					&auth.Payload{
						ID:           "1",
						ProjectID:    "-",
						Names:        "process_stk_worker",
						PhoneNumber:  "",
						EmailAddress: "",
						Group:        auth.DefaultAdminGroup(),
						Roles:        []string{},
					},
					time.Now().Add(time.Minute),
				)
				if err != nil {
					stkAPI.Logger.Errorf("Failed to generate context: %v", err)
					return
				}

				md := metadata.Pairs(auth.Header(), fmt.Sprintf("%s %s", auth.Scheme(), token))

				// Communication context
				ctx := metadata.NewIncomingContext(context.Background(), md)

				// Authorize the context
				ctx, err = stkAPI.AuthAPI.Authenticator(ctx)
				if err != nil {
					stkAPI.Logger.Errorf("Failed to authorize context: %v", err)
					return
				}

				// Lower the processed state
				datas[1] = strings.ToLower(datas[1])

				// Process transaction
				_, err = stkAPI.ProcessStkTransaction(ctx, &stk.ProcessStkTransactionRequest{
					MpesaReceiptId: datas[0],
					Processed:      datas[1] == "true" || datas[1] == "yes",
				})
				if err != nil {
					stkAPI.Logger.Errorf("Failed to process transaction: %v", err)
					return
				}

				stkAPI.Logger.Infof("Successfully processed transaction: %v", datas[0])
			}(msg)
		}
	}
}
