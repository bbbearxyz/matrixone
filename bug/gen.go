package main

import (
	"math/rand"
	"os"
	"time"
)

func GetRandomString(l int) string {
	str := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := []byte(str)
	var result []byte
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < l; i++ {
		result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}

func main() {
	f, _ := os.Create("input")
	defer f.Close()
	f.WriteString("create table t(t varchar(1073741824));\n")
	// f.WriteString("create table t(t text);\n")
	f.WriteString("insert into t values(\"" + GetRandomString(900*1024*1024) + "\");")
}
