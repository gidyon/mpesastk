package stk

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gidyon/gomicro/utils/errs"
	stk "github.com/gidyon/mpesastk/pkg/api/stk/v1"
	"github.com/spf13/viper"
)

// STKTransaction contains mpesa stk transaction details
type STKTransaction struct {
	ID                         uint           `gorm:"primaryKey;autoIncrement"`
	InitiatorID                string         `gorm:"index;type:varchar(50)"`
	InitiatorCustomerReference string         `gorm:"index;type:varchar(50)"`
	InitiatorCustomerNames     string         `gorm:"type:varchar(50)"`
	PhoneNumber                string         `gorm:"index;type:varchar(15);not null"`
	Amount                     string         `gorm:"type:float(10);not null"`
	ShortCode                  string         `gorm:"index;type:varchar(15)"`
	AccountReference           string         `gorm:"index;type:varchar(50)"`
	TransactionDesc            sql.NullString `gorm:"type:varchar(300)"`
	MerchantRequestID          sql.NullString `gorm:"index;type:varchar(50);"`
	CheckoutRequestID          sql.NullString `gorm:"index;type:varchar(50);"`
	StkResponseDescription     sql.NullString `gorm:"type:varchar(300)"`
	StkResponseCustomerMessage sql.NullString `gorm:"type:varchar(300)"`
	StkResponseCode            sql.NullString `gorm:"index;type:varchar(10)"`
	ResultCode                 sql.NullString `gorm:"index;type:varchar(10)"`
	ResultDescription          sql.NullString `gorm:"type:varchar(300)"`
	MpesaReceiptId             sql.NullString `gorm:"index;type:varchar(50);unique"`
	StkStatus                  sql.NullString `gorm:"index;type:varchar(30)"`
	Source                     sql.NullString `gorm:"index;type:varchar(30)"`
	Tag                        sql.NullString `gorm:"index;type:varchar(30)"`
	// Succeeded                  bool         `gorm:"index;type:tinyint(1)"`
	// Processed                  bool         `gorm:"index;type:tinyint(1)"`
	Succeeded       string       `gorm:"index;type:enum('YES','NO');default:NO"`
	Processed       string       `gorm:"index;type:enum('YES','NO');default:NO"`
	TransactionTime sql.NullTime `gorm:"index;type:datetime(6)"`
	UpdatedAt       time.Time    `gorm:"autoUpdateTime;type:datetime(6)"`
	CreatedAt       time.Time    `gorm:"index;autoCreateTime;primaryKey;type:datetime(6);not null"`
}

// StkTable is table for mpesa payments
const StkTable = "stk_transactions"

// TableName returns the name of the table
func (*STKTransaction) TableName() string {
	// Get table prefix
	if viper.GetString("STK_TABLE_PREFIX") != "" {
		return fmt.Sprintf("%s_%s", viper.GetString("STK_TABLE_PREFIX"), StkTable)
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
		TransactionDesc:            db.TransactionDesc.String,
		MerchantRequestId:          db.MerchantRequestID.String,
		CheckoutRequestId:          db.CheckoutRequestID.String,
		StkResponseDescription:     db.StkResponseDescription.String,
		StkResponseCode:            db.StkResponseCode.String,
		StkResultCode:              db.ResultCode.String,
		StkResultDesc:              db.ResultDescription.String,
		MpesaReceiptId:             db.MpesaReceiptId.String,
		Balance:                    "",
		Status:                     stk.StkStatus(stk.StkStatus_value[db.StkStatus.String]),
		Source:                     db.Source.String,
		Tag:                        db.Tag.String,
		Succeeded:                  db.Succeeded == "YES",
		Processed:                  db.Processed == "YES",
		TransactionTimestamp:       db.TransactionTime.Time.UTC().Unix(),
		CreateTimestamp:            db.CreatedAt.UTC().Unix(),
	}

	return pb, nil
}
