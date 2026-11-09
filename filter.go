package mgdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// jakarta dimuat sekali. Jika tzdata tidak tersedia di server, dipakai WIB (UTC+7, tanpa DST)
// supaya time.In tidak pernah menerima nil dan panic.
var jakarta = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// HolidayAPIURL adalah endpoint daftar libur nasional per bulan (?month=N).
var HolidayAPIURL = "https://dayoffapi.vercel.app/api"

// HTTPClient dipakai oleh Get dan HariLibur. Timeout mencegah request menggantung.
var HTTPClient = &http.Client{Timeout: 15 * time.Second}

// maxResponseBytes membatasi ukuran body respons yang dibaca Get (10 MiB).
const maxResponseBytes = 10 << 20

// holidayCacheTTL adalah lama daftar libur per bulan disimpan sebelum diambil ulang.
const holidayCacheTTL = 12 * time.Hour

type holidayEntry struct {
	dates   map[string]bool
	fetched time.Time
}

var (
	holidayMu    sync.Mutex
	holidayCache = map[int]holidayEntry{}
)

func GetDateSekarang() (datesekarang time.Time) {
	t := time.Now().In(jakarta)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, jakarta)
}

// dayRangeFilter membuat filter _id (ObjectID) untuk rentang [start, start+24 jam).
func dayRangeFilter(start time.Time) bson.M {
	return bson.M{
		"$gte": primitive.NewObjectIDFromTimestamp(start),
		"$lt":  primitive.NewObjectIDFromTimestamp(start.Add(24 * time.Hour)),
	}
}

func TodayFilter() bson.M {
	return dayRangeFilter(GetDateSekarang())
}

func YesterdayNotLiburFilter() bson.M {
	return dayRangeFilter(GetDateKemarinBukanHariLibur())
}

func YesterdayFilter() bson.M {
	return dayRangeFilter(GetDateKemarin())
}

func GetDateKemarinBukanHariLibur() (datekemarinbukanlibur time.Time) {
	t := time.Now().In(jakarta).AddDate(0, 0, -1)
	// dibatasi 31 hari supaya tidak pernah berputar tanpa henti
	for i := 0; i < 31 && HariLibur(t); i++ {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, jakarta)
}

func GetDateKemarin() (datekemarin time.Time) {
	t := time.Now().In(jakarta).AddDate(0, 0, -1)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, jakarta)
}

// HariLibur mengembalikan true untuk Sabtu, Minggu, atau tanggal libur nasional.
// Sabtu/Minggu tidak memanggil API. Daftar libur per bulan di-cache selama holidayCacheTTL.
// Jika API gagal, tanggal dianggap bukan libur (perilaku sama seperti sebelumnya).
func HariLibur(thedate time.Time) (libur bool) {
	if wd := thedate.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return true
	}
	return holidaysForMonth(int(thedate.Month()))[thedate.Format("2006-01-02")]
}

func holidaysForMonth(month int) map[string]bool {
	holidayMu.Lock()
	defer holidayMu.Unlock()
	if e, ok := holidayCache[month]; ok && time.Since(e.fetched) < holidayCacheTTL {
		return e.dates
	}
	dates := map[string]bool{}
	_, hasil, err := Get[[]NewLiburNasional](HolidayAPIURL + "?month=" + strconv.Itoa(month))
	if err != nil {
		return dates // jangan cache kegagalan, coba lagi di panggilan berikutnya
	}
	for _, v := range hasil {
		dates[v.Tanggal] = true
	}
	holidayCache[month] = holidayEntry{dates: dates, fetched: time.Now()}
	return dates
}

// Get mengambil URL dan mendekode JSON ke T, dengan timeout dan batas ukuran body.
func Get[T any](urltarget string) (statusCode int, result T, err error) {
	resp, err := HTTPClient.Get(urltarget)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return
	}
	if len(body) > maxResponseBytes {
		err = fmt.Errorf("response from %s exceeds %d bytes", urltarget, maxResponseBytes)
		return
	}
	if er := json.Unmarshal(body, &result); er != nil {
		err = errors.New("Not A Valid JSON Response from " + urltarget + " . CONTENT: " + snippet(body))
		return
	}
	return
}

// snippet memotong body untuk pesan error supaya log tidak membengkak.
func snippet(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "...(truncated)"
	}
	return string(b)
}
