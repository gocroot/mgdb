package mgdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// OperationTimeout adalah batas waktu setiap operasi database (termasuk koneksi awal).
// Tanpa batas, query yang menggantung tetap ditagih di Cloud Functions sampai timeout function.
var OperationTimeout = 30 * time.Second

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), OperationTimeout)
}

// srvResolver dipakai saat resolver sistem tidak bisa membaca record SRV (misalnya di beberapa hosting).
var srvResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, "8.8.8.8:53")
	},
}

// MongoConnect membuat koneksi ke MongoDB. Jika URI mongodb+srv:// gagal di-resolve oleh
// resolver sistem, record SRV/TXT dicari lewat DNS 8.8.8.8 lalu koneksi diulang.
// Tidak pernah panic: semua kegagalan dikembalikan sebagai error.
func MongoConnect(mconn DBInfo) (db *mongo.Database, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mconn.DBString))
	if err != nil && strings.HasPrefix(mconn.DBString, "mongodb+srv://") {
		uri, lerr := LookupSRV(mconn.DBString)
		if lerr != nil {
			return nil, fmt.Errorf("%w (fallback SRV lookup: %v)", err, lerr)
		}
		client, err = mongo.Connect(ctx, options.Client().ApplyURI(uri))
	}
	if err != nil {
		return nil, err
	}
	return client.Database(mconn.DBName), nil
}

// LookupSRV mengubah URI mongodb+srv:// menjadi URI mongodb:// berisi daftar host dari
// record SRV dan opsi dari record TXT, memakai DNS 8.8.8.8.
func LookupSRV(srvuri string) (string, error) {
	u, err := url.Parse(srvuri)
	if err != nil {
		return "", fmt.Errorf("mgdb: URI tidak valid: %w", err)
	}
	if u.Scheme != "mongodb+srv" || u.Hostname() == "" {
		return "", errors.New("mgdb: URI harus berbentuk mongodb+srv://user:pass@host/db")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, srvs, err := srvResolver.LookupSRV(ctx, "mongodb", "tcp", u.Hostname())
	if err != nil {
		return "", fmt.Errorf("mgdb: lookup SRV %s: %w", u.Hostname(), err)
	}
	if len(srvs) == 0 {
		return "", fmt.Errorf("mgdb: tidak ada record SRV untuk %s", u.Hostname())
	}
	txts, _ := srvResolver.LookupTXT(ctx, u.Hostname()) // TXT opsional
	return buildSRVURI(u, srvs, txts), nil
}

// buildSRVURI menyusun URI mongodb:// dari hasil lookup. Opsi di URI asli menimpa opsi dari TXT.
func buildSRVURI(u *url.URL, srvs []*net.SRV, txts []string) string {
	hosts := make([]string, 0, len(srvs))
	for _, s := range srvs {
		hosts = append(hosts, net.JoinHostPort(strings.TrimSuffix(s.Target, "."), strconv.Itoa(int(s.Port))))
	}
	q := url.Values{}
	for _, txt := range txts {
		if v, err := url.ParseQuery(txt); err == nil {
			for k, vs := range v {
				q[k] = vs
			}
		}
	}
	for k, vs := range u.Query() {
		q[k] = vs
	}
	if q.Get("tls") == "" && q.Get("ssl") == "" {
		q.Set("tls", "true") // mongodb+srv selalu memakai TLS
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	out := url.URL{Scheme: "mongodb", User: u.User, Host: strings.Join(hosts, ","), Path: path, RawQuery: q.Encode()}
	return out.String()
}

// SRVLookup mengembalikan string kosong jika gagal (versi lama panic).
//
// Deprecated: pakai LookupSRV yang mengembalikan error.
func SRVLookup(srvuri string) (mongouri string) {
	mongouri, _ = LookupSRV(srvuri)
	return
}

func GetAllDistinctDoc(db *mongo.Database, filter bson.M, fieldname, collection string) (doc []any, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	doc, err = db.Collection(collection).Distinct(ctx, fieldname, filter)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// GetAllDistinctDoc mengambil semua nilai yang berbeda dari field tertentu dalam koleksi yang diberikan
func GetAllDistinct[T any](db *mongo.Database, filter bson.M, fieldname, collection string) ([]T, error) {
	ctx, cancel := opCtx()
	defer cancel()
	rawDoc, err := db.Collection(collection).Distinct(ctx, fieldname, filter)
	if err != nil {
		return nil, err
	}

	// Mengkonversi []interface{} ke []T
	result := make([]T, len(rawDoc))
	for i, v := range rawDoc {
		value, ok := v.(T)
		if !ok {
			return nil, fmt.Errorf("type assertion to %T failed", v)
		}
		result[i] = value
	}
	return result, nil
}

func GetRandomDoc[T any](db *mongo.Database, collection string, size uint) (result []T, err error) {
	filter := mongo.Pipeline{
		{{Key: "$sample", Value: bson.D{{Key: "size", Value: size}}}},
	}
	ctx, cancel := opCtx()
	defer cancel()
	cursor, err := db.Collection(collection).Aggregate(ctx, filter)
	if err != nil {
		return
	}

	err = cursor.All(ctx, &result)

	return
}

func GetAllDoc[T any](db *mongo.Database, collection string, filter bson.M) (doc T, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	cur, err := db.Collection(collection).Find(ctx, filter)
	if err != nil {
		return
	}
	defer cur.Close(ctx)
	err = cur.All(ctx, &doc)
	if err != nil {
		return
	}
	return
}

func GetCountDoc(db *mongo.Database, collection string, filter bson.M) (count int64, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	count, err = db.Collection(collection).CountDocuments(ctx, filter)
	if err != nil {
		return
	}
	return
}

func GetOneDoc[T any](db *mongo.Database, collection string, filter bson.M) (doc T, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	err = db.Collection(collection).FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return
	}
	return
}

// Fungsi untuk menghapus koleksi lmsusers
func DropCollection(db *mongo.Database, collection string) error {
	ctx, cancel := opCtx()
	defer cancel()
	return db.Collection(collection).Drop(ctx)
}

func DeleteManyDocs(db *mongo.Database, collection string, filter bson.M) (deleteresult *mongo.DeleteResult, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	deleteresult, err = db.Collection(collection).DeleteMany(ctx, filter)
	return
}

func DeleteOneDoc(db *mongo.Database, collection string, filter bson.M) (updateresult *mongo.DeleteResult, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	updateresult, err = db.Collection(collection).DeleteOne(ctx, filter)
	return
}

func GetOneLatestDoc[T any](db *mongo.Database, collection string, filter bson.M) (doc T, err error) {
	opts := options.FindOne().SetSort(bson.M{"$natural": -1})
	ctx, cancel := opCtx()
	defer cancel()
	err = db.Collection(collection).FindOne(ctx, filter, opts).Decode(&doc)
	if err != nil {
		return
	}
	return
}

func GetOneLowestDoc[T any](db *mongo.Database, collection string, filter bson.M, sortField string) (doc T, err error) {
	opts := options.FindOne().SetSort(bson.M{sortField: 1}) // Sort by the provided field in ascending order
	ctx, cancel := opCtx()
	defer cancel()
	err = db.Collection(collection).FindOne(ctx, filter, opts).Decode(&doc)
	if err != nil {
		return
	}
	return
}

func InsertOneDoc(db *mongo.Database, collection string, doc interface{}) (insertedID primitive.ObjectID, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	insertResult, err := db.Collection(collection).InsertOne(ctx, doc)
	if err != nil {
		return
	}
	// Dokumen dengan _id bukan ObjectID (misalnya string) tetap tersimpan, tapi tidak boleh membuat panic.
	id, ok := insertResult.InsertedID.(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("mgdb: dokumen tersimpan, tapi InsertedID bertipe %T, bukan ObjectID", insertResult.InsertedID)
	}
	return id, nil
}

// Fungsi untuk menyisipkan banyak dokumen ke dalam koleksi: insertedIDs, err := InsertManyDocs(db, collection, docs)
func InsertManyDocs[T any](db *mongo.Database, collection string, docs []T) (insertedIDs []interface{}, err error) {
	// Konversi []T ke []interface{}
	interfaceDocs := make([]interface{}, len(docs))
	for i, v := range docs {
		interfaceDocs[i] = v
	}

	ctx, cancel := opCtx()
	defer cancel()
	insertResult, err := db.Collection(collection).InsertMany(ctx, interfaceDocs)
	if err != nil {
		return nil, err
	}
	return insertResult.InsertedIDs, nil
}

// With UpdateOneDoc() allows for updating fields, new fields can be added without losing the fields in the old document.
//
// Perhatian: UpdateOneDoc memakai upsert, jadi jika filter tidak cocok dengan dokumen apa pun,
// dokumen baru akan dibuat. Jangan bangun filter langsung dari input user tanpa validasi.
//
//	updatefields := bson.M{
//		"token":         token.AccessToken,
//		"refresh_token": token.RefreshToken,
//		"expiry":        token.Expiry,
//	}
func UpdateOneDoc(db *mongo.Database, collection string, filter bson.M, updatefields bson.M) (updateresult *mongo.UpdateResult, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	updateresult, err = db.Collection(collection).UpdateOne(ctx, filter, bson.M{"$set": updatefields}, options.Update().SetUpsert(true))
	if err != nil {
		return
	}
	return
}

// With ReplaceOneDoc() you can only replace the entire document,
// while UpdateOneDoc() allows for updating fields. Since ReplaceOneDoc() replaces the entire document - fields in the old document not contained in the new will be lost.
func ReplaceOneDoc(db *mongo.Database, collection string, filter bson.M, doc interface{}) (updatereseult *mongo.UpdateResult, err error) {
	ctx, cancel := opCtx()
	defer cancel()
	updatereseult, err = db.Collection(collection).ReplaceOne(ctx, filter, doc)
	if err != nil {
		return
	}
	return
}
