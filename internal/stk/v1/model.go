package stk

import (
	"database/sql"
	"fmt"
	"time"

	stk "bitbucket.org/gideonkamau/mpesastk/pkg/api/stk/v1"
	"github.com/gidyon/gomicro/utils/errs"
	"github.com/spf13/viper"
)

func init() {
	tablePrefix = viper.GetString("STK_TABLE_PREFIX")
}

// STKTransaction contains mpesa stk transaction details
type STKTransaction struct {
	ID                         uint         `gorm:"primaryKey;autoIncrement"`
	InitiatorID                string       `gorm:"index;type:varchar(50)"`
	InitiatorCustomerReference string       `gorm:"index;type:varchar(50)"`
	InitiatorCustomerNames     string       `gorm:"type:varchar(50)"`
	PhoneNumber                string       `gorm:"index;type:varchar(15);not null"`
	Amount                     string       `gorm:"type:float(10);not null"`
	ShortCode                  string       `gorm:"index;type:varchar(15)"`
	AccountReference           string       `gorm:"index;type:varchar(50)"`
	TransactionDesc            string       `gorm:"type:varchar(300)"`
	MerchantRequestID          string       `gorm:"index;type:varchar(50);"`
	CheckoutRequestID          string       `gorm:"index;type:varchar(50);"`
	StkResponseDescription     string       `gorm:"type:varchar(300)"`
	StkResponseCustomerMessage string       `gorm:"type:varchar(300)"`
	StkResponseCode            string       `gorm:"index;type:varchar(10)"`
	ResultCode                 string       `gorm:"index;type:varchar(10)"`
	ResultDescription          string       `gorm:"type:varchar(300)"`
	MpesaReceiptId             string       `gorm:"index;type:varchar(50);unique"`
	StkStatus                  string       `gorm:"index;type:varchar(30)"`
	Source                     string       `gorm:"index;type:varchar(30)"`
	Tag                        string       `gorm:"index;type:varchar(30)"`
	Succeeded                  bool         `gorm:"index;type:tinyint(1)"`
	Processed                  bool         `gorm:"index;type:tinyint(1)"`
	TransactionTime            sql.NullTime `gorm:"index;type:datetime(6)"`
	UpdatedAt                  time.Time    `gorm:"autoUpdateTime;type:datetime(6)"`
	CreatedAt                  time.Time    `gorm:"index;autoCreateTime;primaryKey;type:datetime(6);not null"`
}

// StkTable is table for mpesa payments
const StkTable = "stk_transactions"

var tablePrefix = ""

// TableName returns the name of the table
func (*STKTransaction) TableName() string {
	// Get table prefix
	if tablePrefix != "" {
		return fmt.Sprintf("%s_%s", tablePrefix, StkTable)
	}
	return StkTable
}

// ToProto returns the protobuf message of stk transaction
func ToProto(db *STKTransaction) (*stk.StkTransaction, error) {
	if db == nil {
		return nil, errs.MissingField("stk payment")
	}

	pb := &stk.StkTransaction{
		InitiatorId:                db.InitiatorID,
		TransactionId:              uint64(db.ID),
		InitiatorCustomerReference: db.InitiatorCustomerReference,
		InitiatorCustomerNames:     db.InitiatorCustomerNames,
		ShortCode:                  db.ShortCode,
		AccountReference:           db.AccountReference,
		Amount:                     db.Amount,
		PhoneNumber:                db.PhoneNumber,
		TransactionDesc:            db.TransactionDesc,
		MerchantRequestId:          db.MerchantRequestID,
		CheckoutRequestId:          db.CheckoutRequestID,
		StkResponseDescription:     db.StkResponseDescription,
		StkResponseCode:            db.StkResponseCode,
		StkResultCode:              db.ResultCode,
		StkResultDesc:              db.ResultDescription,
		MpesaReceiptId:             db.MpesaReceiptId,
		Balance:                    "",
		Status:                     stk.StkStatus(stk.StkStatus_value[db.StkStatus]),
		Source:                     db.Source,
		Tag:                        db.Tag,
		Succeeded:                  db.Succeeded,
		Processed:                  db.Processed,
		TransactionTimestamp:       db.TransactionTime.Time.UTC().Unix(),
		CreateTimestamp:            db.CreatedAt.UTC().Unix(),
	}

	return pb, nil
}
