# MongoDB Helper for Golang

## Overview
This package (`mgdb`) provides a set of helper functions to simplify interaction with MongoDB from a Golang application. It includes connection handling, querying, inserting, updating, deleting documents, and working with distinct and random documents.

## Installation
To use this package, install it using:
```sh
go get github.com/gocroot/mgdb
```

Then, include the package in your Go project:
```go
import "github.com/gocroot/mgdb"
```

## Connecting to MongoDB
### Example:
```go
mconn := mgdb.DBInfo{
    DBString: "mongodb+srv://username:password@cluster.mongodb.net/mydatabase",
    DBName:   "mydatabase",
}
db, err := mgdb.MongoConnect(mconn)
if err != nil {
    log.Fatal(err)
}
```

## CRUD Operations

### Insert Documents
#### Insert One Document
```go
doc := bson.M{"name": "John Doe", "age": 30}
id, err := mgdb.InsertOneDoc(db, "users", doc)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Inserted ID:", id)
```

#### Insert Multiple Documents
```go
docs := []bson.M{
    {"name": "Alice", "age": 25},
    {"name": "Bob", "age": 28},
}
ids, err := mgdb.InsertManyDocs(db, "users", docs)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Inserted IDs:", ids)
```

### Query Documents
#### Get One Document
```go
filter := bson.M{"name": "John Doe"}
user, err := mgdb.GetOneDoc[struct {
    Name string `bson:"name"`
    Age  int    `bson:"age"`
}](db, "users", filter)
if err != nil {
    log.Fatal(err)
}
fmt.Println("User:", user)
```

#### Get All Documents
```go
filter := bson.M{} // Empty filter to get all documents
users, err := mgdb.GetAllDoc[[]struct{Name string `bson:"name"`; Age int `bson:"age"`}](db, "users", filter)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Users:", users)
```

#### Get Distinct Field Values
```go
values, err := mgdb.GetAllDistinct[string](db, bson.M{}, "name", "users")
if err != nil {
    log.Fatal(err)
}
fmt.Println("Distinct Names:", values)
```

#### Get Random Documents
```go
randomUsers, err := mgdb.GetRandomDoc[struct{Name string `bson:"name"`; Age int `bson:"age"`}](db, "users", 2)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Random Users:", randomUsers)
```

### Update Documents
#### Update One Document
```go
filter := bson.M{"name": "John Doe"}
updateFields := bson.M{"age": 31}
_, err := mgdb.UpdateOneDoc(db, "users", filter, updateFields)
if err != nil {
    log.Fatal(err)
}
```

#### Replace One Document
```go
newUser := struct {
    Name string `bson:"name"`
    Age  int    `bson:"age"`
    City string `bson:"city"`
}{"John Doe", 32, "New York"}

_, err := mgdb.ReplaceOneDoc(db, "users", filter, newUser)
if err != nil {
    log.Fatal(err)
}
```

### Delete Documents
#### Delete One Document
```go
_, err := mgdb.DeleteOneDoc(db, "users", filter)
if err != nil {
    log.Fatal(err)
}
```

#### Delete Multiple Documents
```go
_, err := mgdb.DeleteManyDocs(db, "users", bson.M{"age": bson.M{"$lt": 30}})
if err != nil {
    log.Fatal(err)
}
```

#### Drop Collection
```go
err := mgdb.DropCollection(db, "users")
if err != nil {
    log.Fatal(err)
}
```

### Working with Array Fields
#### Add Document to Array
```go
type Hobby struct {
    Name string `bson:"name"`
}

newHobby := Hobby{Name: "Cycling"}
_, err := mgdb.AddDocToArray[Hobby](db, "users", userID, "hobbies", newHobby)
if err != nil {
    log.Fatal(err)
}
```

#### Remove Document from Array
```go
hobbyToRemove := Hobby{Name: "Cycling"}
_, err := mgdb.DeleteDocFromArray[Hobby](db, "users", userID, "hobbies", hobbyToRemove)
if err != nil {
    log.Fatal(err)
}
```

#### Edit Document in Array
```go
filterCondition := bson.M{"name": "Cycling"}
updatedFields := bson.M{"name": "Mountain Biking"}
_, err := mgdb.EditDocInArray(db, "users", userID, "hobbies", filterCondition, updatedFields)
if err != nil {
    log.Fatal(err)
}
```

### Date and Time Utilities
#### Get Yesterday Start and End Time
```go
startDay, endDay := mgdb.GetYesterdayStartEnd()
```

#### Get Current Date
```go
dateNow := mgdb.GetDateSekarang()
```

#### Filters for Dates
```go
todayFilter := mgdb.TodayFilter()
yesterdayFilter := mgdb.YesterdayFilter()
yesterdayNotHolidayFilter := mgdb.YesterdayNotLiburFilter()
```

#### Get Yesterday's Date (Excluding Holidays)
```go
yesterdayNonHoliday := mgdb.GetDateKemarinBukanHariLibur()
```

#### Check if a Date is a Holiday
```go
isHoliday := mgdb.HariLibur(time.Now())
```

## License
This package is open-source and available for use under the MIT License.

## Release Version
```sh
go get -u all
go mod tidy
git tag v0.1.1
git push origin --tags
go list -m github.com/gocroot/mgdb@v0.1.1
```