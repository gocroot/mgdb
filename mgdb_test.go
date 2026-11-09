package mgdb

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestBuildSRVURI(t *testing.T) {
	u, _ := url.Parse("mongodb+srv://john:p%40ss@cluster0.abc.mongodb.net/mydb?retryWrites=true&w=majority")
	srvs := []*net.SRV{
		{Target: "shard-00-00.abc.mongodb.net.", Port: 27017},
		{Target: "shard-00-01.abc.mongodb.net.", Port: 27017},
	}
	got := buildSRVURI(u, srvs, []string{"authSource=admin&replicaSet=atlas-xyz"})

	want := []string{
		"mongodb://john:p%40ss@shard-00-00.abc.mongodb.net:27017,shard-00-01.abc.mongodb.net:27017/mydb?",
		"authSource=admin", "replicaSet=atlas-xyz", "retryWrites=true", "w=majority", "tls=true",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("URI %q tidak mengandung %q", got, w)
		}
	}
	if strings.Count(got, "?") != 1 {
		t.Errorf("URI harus punya tepat satu '?': %q", got)
	}
	// URI hasil harus bisa diterima driver MongoDB
	if err := options.Client().ApplyURI(got).Validate(); err != nil {
		t.Errorf("driver menolak URI %q: %v", got, err)
	}

	// tanpa nama database tetap valid
	u2, _ := url.Parse("mongodb+srv://u:p@c.example.net")
	got2 := buildSRVURI(u2, srvs[:1], nil)
	if !strings.HasPrefix(got2, "mongodb://u:p@shard-00-00.abc.mongodb.net:27017/?") {
		t.Errorf("tanpa db: %q", got2)
	}
}

func TestLookupSRVInvalidNoPanic(t *testing.T) {
	for _, in := range []string{"", "mongodb+srv://", "bukan uri", "mongodb://u:p@h/db", "mongodb+srv://u:p@/db", "::::"} {
		if _, err := LookupSRV(in); err == nil {
			t.Errorf("LookupSRV(%q): expected error", in)
		}
		if got := SRVLookup(in); got != "" { // dulu panic
			t.Errorf("SRVLookup(%q) = %q, want kosong", in, got)
		}
	}
}

func TestMongoConnectInvalidURINoPanic(t *testing.T) {
	for _, in := range []string{"", "bukan-uri", "mongodb+srv://"} {
		if _, err := MongoConnect(DBInfo{DBString: in, DBName: "x"}); err == nil {
			t.Errorf("MongoConnect(%q): expected error", in)
		}
	}
}

func TestEditDocInArrayRejectsOperatorKeys(t *testing.T) {
	// validasi terjadi sebelum menyentuh database, jadi db nil aman di sini
	if _, err := EditDocInArray(nil, "c", [12]byte{}, "arr", map[string]any{"$where": "1"}, map[string]any{"a": 1}); err == nil {
		t.Error("key filter berawalan $ harus ditolak")
	}
	if _, err := EditDocInArray(nil, "c", [12]byte{}, "arr", map[string]any{"a": 1}, map[string]any{"$set": 1}); err == nil {
		t.Error("key update berawalan $ harus ditolak")
	}
}

func withHolidayServer(t *testing.T, body string) *int32 {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(body))
	}))
	oldURL := HolidayAPIURL
	HolidayAPIURL = srv.URL
	holidayMu.Lock()
	holidayCache = map[int]holidayEntry{}
	holidayMu.Unlock()
	t.Cleanup(func() {
		srv.Close()
		HolidayAPIURL = oldURL
		holidayMu.Lock()
		holidayCache = map[int]holidayEntry{}
		holidayMu.Unlock()
	})
	return &calls
}

func TestHariLiburWeekendAndCache(t *testing.T) {
	calls := withHolidayServer(t, `[{"tanggal":"2026-08-17","keterangan":"HUT RI"}]`)

	if !HariLibur(time.Date(2026, 8, 15, 0, 0, 0, 0, jakarta)) { // Sabtu
		t.Error("Sabtu harus libur")
	}
	if *calls != 0 {
		t.Errorf("Sabtu/Minggu tidak boleh memanggil API, calls=%d", *calls)
	}
	if !HariLibur(time.Date(2026, 8, 17, 0, 0, 0, 0, jakarta)) { // Senin, HUT RI
		t.Error("17 Agustus harus libur")
	}
	if HariLibur(time.Date(2026, 8, 18, 0, 0, 0, 0, jakarta)) { // Selasa biasa
		t.Error("18 Agustus bukan libur")
	}
	if *calls != 1 {
		t.Errorf("bulan yang sama harus dari cache, calls=%d", *calls)
	}
}

func TestGetLimitsAndTruncates(t *testing.T) {
	big := strings.Repeat("x", 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(big))
	}))
	defer srv.Close()
	_, _, err := Get[map[string]any](srv.URL)
	if err == nil || len(err.Error()) > 1024 || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error harus ringkas dan terpotong, panjang=%d", len(err.Error()))
	}
}

func TestJakartaNeverNil(t *testing.T) {
	if jakarta == nil {
		t.Fatal("lokasi Jakarta nil")
	}
	_ = GetDateSekarang()
	_ = TodayFilter()
	_ = YesterdayFilter()
	_, _ = GetYesterdayStartEnd()
}
