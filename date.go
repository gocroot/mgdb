package mgdb

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func GetYesterdayStartEnd() (startDay, endDay primitive.ObjectID) {
	// Hitung tanggal kemarin (zona Asia/Jakarta, dimuat sekali di filter.go)
	yesterday := time.Now().In(jakarta).AddDate(0, 0, -1)
	startOfDay := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, jakarta)
	endOfDay := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 999999999, jakarta)
	startDay = primitive.NewObjectIDFromTimestamp(startOfDay)
	endDay = primitive.NewObjectIDFromTimestamp(endOfDay)
	return
}
