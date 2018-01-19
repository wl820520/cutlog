package main

import (
	"io"
	"net/http"
	"log"
	"time"
	"strings"
	"bytes"
	"encoding/hex"
	"crypto/md5"
	"math/rand"
	"runtime"
	"./cutlog"
)
func TestLog(w http.ResponseWriter, req *http.Request) {
	cutlog.StartProvider()
	req.ParseForm()
	if req.Form["req"]!=nil {
		//var str string= req.Form["req"][0]
		logstr := "测试文件日志"
		b:=bytes.Buffer{}
		for i:=0;i<1000;i++{
			b.WriteString(logstr)
		}
		//println(logstr)
		logType:=strings.Split(logstr,",")
		println(logType)
		cutlog.Println(logstr,"Info")
		io.WriteString(w, "成功")
	}else{
		io.WriteString(w, "缺少参数")
	}
}
func InsertLog(w http.ResponseWriter, req *http.Request) {
	logstr := "测试日志"
	b:=bytes.Buffer{}
	//for i:=0;i<1000;i++{
		//j:=strconv.FormatInt(int64(i),32)
		b.WriteString(logstr)
	//}
	runtime.GOMAXPROCS(8) //设置cpu的核的数量，从而实现高并发
	c := make(chan bool)
	for i := 0; i < 10; i++ {
		go writebuf(c, i,b)
	}
	io.WriteString(w, "成功")
}
// 生成32位MD5
func MD5(text string) string{
	ctx := md5.New()
	ctx.Write([]byte(text))
	return hex.EncodeToString(ctx.Sum(nil))
}

// return len=8  salt
func GetRandomSalt() string {
	return GetRandomString(8)
}

//生成随机字符串
func GetRandomString(lenth int64) string{
	str := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	bytes := []byte(str)
	result := []byte{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < int(lenth); i++ {
		result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}
func writebuf(c chan bool, n int,b bytes.Buffer) {
	for i := 10000; i < 15000; i++ {
		j:=GetRandomString(32) //strconv.FormatInt(int64(i),32)
		cutlog.Println(b.String()+j, "Info")
	}
	if n==9{
		c <- true
	}
}
func main() {
	err2:= cutlog.StartProvider()
	if err2!=nil{
		println("日志启动失败"+err2.Error())
	}
	http.HandleFunc("/testlog", TestLog)
	http.HandleFunc("/insertlog", InsertLog)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("端口异常: ", err)
	}
}
