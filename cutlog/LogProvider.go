package cutlog

import (
	"time"
	"sync"
	"os"
	"bytes"
	"log"
	"runtime/debug"
	"goini"
	"io"
	"qiniupkg.com/x/errors.v7"
	"fmt"
	"encoding/json"
)
//@description	日志记录工具类
/*
日志功能：提供自定义多文件多类型日志记录，对外暴露Println方法，main中调用StartProvider对log进行初始化，需在配置文件(config.ini)中配置日志相关文件的属性配置。
配置文件说明：
[Log]   //根节点，必须配置
LoggerType= Debug,Error,Info //自定义文件名称作为根节点的子节点,
FilePath  //文件路径，
DirDataPattern  //是否按日期拆分文件夹  传空不拆
CutType= 1 //分割方式  1：日期，2：文件大小，3：不分割
MaxFileSize= 10000 //最大文件大小
BufferLength	//缓存区大小
MaxChannelSize //通道最大数
DatePattern= 2006010203  //日期格式，(请严格按照time.format方式设置)
Is_debug = true //是否调试模式，调试模式下输出到控制台

[Debug]
Debug_enable= true //是否输出文件  设置为false 不生成文件
Debug_fileName= debug.log //文件名称

日志输入操作	Println换行打印
日志内容：文本数据
日志锁机制：缓冲锁 mu_buf  文件锁 mu_file
日志监听操作
	A.channel数据监听 （队列缓冲作用，定义channel大小，防内存溢出）
		1.读取channel队列数据，写入buffer
		2.判断是否需要重命名(文件大小或日期过期)  判断buffer大小
		3.满足2判断条件写入文件
	B.定时监听：
		1.每5秒将非空buffer写入文件 防日志堆积和长时间无日志入队
*/

//-----const----------------------------------------
const (
	_VERSION_          = "1.0.1"               //版本
	DATEFORMAT         = "2006-01-02"          //日期格式(用于文件命名)
	TIMEFORMAT         = "2006/01/02 15:04:05" //时间格式(日志时间格式)
	_SPACE             = " "                   //参数分割
	_TABLE             = "\t"                  //日志文件行分隔符
	_JOIN              = "&"                   //参数连接符
	_FILE_OPERAT_MODE_ = 0644                  //文件操作权限模式
	_FILE_CREAT_MODE_  = 0666                  //文件建立权限模式
	_LABEL_            = "[_loggor_]"          //标签
)
//------------type struct------------------------------
type logProvider struct {
	fileSerial      map[string]int           //大小分割文件时的文件序号
	maxFileCount int32			//文件最大个数
	fileDate     map[string]*time.Time    //文件时间
	mu_buf       map[string]*sync.Mutex   //缓冲互斥锁
	mu_file      map[string]*sync.Mutex   //文件互斥锁
	logFile      map[string]*os.File      //文件句柄对象
	writeTimer *time.Timer   //写入定时器  超过max秒无日志写buffer数据
	yxpBuffer    map[string]*bytes.Buffer //buffer
	bufLength	int//buffer最大存放日志数
	yxpChannel	map[string]chan string//  channel日志通道
	maxChannelSize int64	//channel最大数
	isDebug bool //是否调试模式，调试模式输出日志到console
	cutType int32 //文件分割模式 1:日期，2：文件大小，3：不分割
	isStart bool //是否启动 防多次启动
}

type yXPSetting struct {
	name string //设置名称  Info,Debug,Error
	use bool //是否记录
	fileDir          string        //目录日志文件路径
	fileName     string        //文件名
	maxFileSize  int64         //文件大小
	maxFileCount int32         //文件个数
	cutType 	int32          //1:日期分割，2：文件大小分割，3：不分割
	datePattern string 	//日期分割格式  20180118
}
//------------全局对象-------------------------------------------------
var yxplog *logProvider=new(logProvider) //日志对象
var settingArr []yXPSetting //全局配置数据
var objConfig *goini.Config

//-----------初始化配置-------------------------------------------
func StartProvider() error  {
	if !yxplog.isStart {
		yxplog.isStart=true
		objConfig = goini.SetConfig("config.ini")
		if objConfig.GetValue("Log","Is_debug")=="true"{
			yxplog.isDebug=true
		}else {
			yxplog.isDebug=false
		}
		setConfig()
		if len(settingArr)==0{
			if yxplog.isDebug{
				log.Println("read config fail, no config file")
			}
			return errors.New("read config fail, no config file")
		}
		err:= initLog()
		if err!=nil{
			return errors.New("initLog fail，"+err.Error())
		}
	}
	return nil
}
func initLog() error {
	yxplog.mu_buf = make(map[string]*sync.Mutex,0)
	yxplog.mu_file = make(map[string]*sync.Mutex,0)
	yxplog.yxpBuffer=make(map[string]*bytes.Buffer,0)
	yxplog.maxFileCount=1000
	yxplog.maxChannelSize=objConfig.GetValueInt64("Log","MaxChannelSize")
	yxplog.bufLength=objConfig.GetValueInt("Log","BufferLength")
	checkFileDir()
	yxplog.logFile=make(map[string]*os.File,0)
	yxplog.yxpChannel=make(map[string]chan string,yxplog.maxChannelSize)
	yxplog.fileSerial=make(map[string]int,0)
	yxplog.fileDate =make(map[string]*time.Time,0)
	for _,v:=range settingArr{
		yxplog.yxpChannel[v.name]=make(chan string,yxplog.maxChannelSize)
		yxplog.fileSerial[v.name]=0
		yxplog.fileDate[v.name]= getNowFormDate(v.datePattern)
		yxplog.mu_buf[v.name]=new(sync.Mutex)
		yxplog.mu_file[v.name]=new(sync.Mutex)
		yxplog.yxpBuffer[v.name]=new(bytes.Buffer)
		tp:=getFilePath(v)
		file, err := os.OpenFile(
			tp,
			os.O_RDWR|os.O_APPEND|os.O_CREATE,
			_FILE_OPERAT_MODE_,
		)
		if err!=nil{
			return err
		}
		yxplog.logFile[v.name]=file
	}
	//消费channel
	for _,v:=range settingArr {
		go  runChannel(v)
		go  runTime()
	}
	if yxplog.isDebug {
		jstr, err := json.Marshal(yxplog)
		if nil == err {
			log.Println(string(jstr))
		}
	}
	return nil
}
func runTime()  {
	yxplog.writeTimer = time.NewTimer(5*time.Second)
	for {
		select {
		case <-yxplog.writeTimer.C:
			for _,v:=range settingArr{
				writeBuffer(v,yxplog.yxpBuffer[v.name])
			}
			yxplog.writeTimer.Reset(5*time.Second)
			break
		}
	}
}
func runChannel(v yXPSetting)  {
	for {
		if log, ok := <-yxplog.yxpChannel[v.name]; ok {
			yxplog.yxpBuffer[v.name].WriteString(log)
			if checkFile(v)||yxplog.yxpBuffer[v.name].Len()>yxplog.bufLength{
				writeBuffer(v,yxplog.yxpBuffer[v.name])
			}
			continue
		}
	}
}
func checkFile(v yXPSetting)bool {
	defer func() {
		if e, ok := recover().(error); ok {
			log.Println(_LABEL_, "WARN: panic - %v", e)
			log.Println(_LABEL_, string(debug.Stack()))
		}
	}()
	var IS_RENAME bool = false
	switch yxplog.cutType {
	case 1:
		now := getNowFormDate(v.datePattern)
		if nil != now &&
			nil != yxplog.fileDate[v.name] &&
			now.After(*yxplog.fileDate[v.name]) {
			IS_RENAME = true
		}
	case 2:
		filesize:=fileSize(getFilePath(v))
		if  filesize>= v.maxFileSize {
			IS_RENAME = true
		}
	case 3:
		IS_RENAME = false
	}
	if IS_RENAME {
		yxplog.mu_file[v.name].Lock()
		defer yxplog.mu_file[v.name].Unlock()
		if yxplog.isDebug {
			log.Println(_LABEL_, getFilePath(v), " is need rename.")
		}
		renameFile(v)
	}
	return IS_RENAME
}

func fileSize(file string) int64 {
	this, e := os.Stat(file)
	if e != nil {
		if yxplog.isDebug {
			log.Println(_LABEL_, e.Error())
		}
		return 0
	}

	return this.Size()
}

func  renameFile(set yXPSetting) {
	var err error
	defer func() {
		if yxplog.isDebug {
			log.Println("renameFile fail ")
		}
	}()
	oldName := getFilePath(set)
	yxplog.fileSerial[set.name] = yxplog.fileSerial[set.name] + 1
	newName := getFilePath(set)
	yxplog.logFile[set.name].Close()
	if "" != oldName && "" != newName && oldName != newName {
		if isExist(newName) {
			err := os.Remove(newName)
			if nil != err {
				log.Println(_LABEL_, "del file fail", err.Error())
			}
		}
	}
	yxplog.logFile[set.name], err = os.OpenFile(
		newName,
		os.O_RDWR|os.O_APPEND|os.O_CREATE,
		_FILE_OPERAT_MODE_,
	)
	if err != nil {
		log.Println(_LABEL_, "add new file fail", err.Error())
	}
	return
}

func  writeBuffer(v yXPSetting,buf *bytes.Buffer) {
	if buf.Len()<=0{
		return
	}
	yxplog.mu_file[v.name].Lock()
	defer yxplog.mu_file[v.name].Unlock()
	yxplog.mu_buf[v.name].Lock()
	defer yxplog.mu_buf[v.name].Unlock()
	_, err := io.WriteString(yxplog.logFile[v.name], buf.String())
	if nil != err {
		yxplog.logFile[v.name], err = os.OpenFile(
			getFilePath(v),
			os.O_RDWR | os.O_APPEND | os.O_CREATE,
			_FILE_OPERAT_MODE_,
		)
		if nil != err {
			log.Println(_LABEL_, "log bufWrite() err!")
		}
	}
	yxplog.yxpBuffer[v.name]=new(bytes.Buffer)
}
func getNowFormDate(form string) *time.Time {
	t, err := time.Parse(form, time.Now().Format(form))
	if nil != err {
		log.Println(_LABEL_, "getNowFormDate()", err.Error())
		t = time.Time{}
		return &t
	}

	return &t
}
func isExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}
func setConfig()  {
	str:= objConfig.GetValueArray("Log", ("LoggerType"))
	filePath:=objConfig.GetValue("Log","FilePath")
	dirDatePattern:=objConfig.GetValue("Log","DirDataPattern")
	if dirDatePattern!=""{
		filePath=filePath+fmt.Sprint(time.Now().Format(dirDatePattern),"\\")
	}
	cutType:=objConfig.GetValueInt32("Log","CutType")
	maxFileSize:=objConfig.GetValueInt64("Log","MaxFileSize")
	datePattern:=objConfig.GetValue("Log","DatePattern")
	yxplog.cutType=cutType
	for _,t:=range str{
		flag:= objConfig.GetValue(t, (t+"_enable"))
		if flag=="true" {
			c := yXPSetting{
				name:t,
				fileDir:filePath,
				fileName:objConfig.GetValue(t, (t+"_fileName")),
				maxFileSize:maxFileSize,
				maxFileCount:1000,
				cutType:cutType,
				datePattern:datePattern,
			}
			settingArr=append(settingArr,c)
		}
	}
}
func getFilePath(v yXPSetting)string  {
	var tp string
	switch v.cutType {
	case 1:tp=fmt.Sprint(v.fileDir,time.Now().Format(v.datePattern),"_",v.fileName)
	case 2:tp=fmt.Sprint(v.fileDir,fmt.Sprintf("%05d", int(yxplog.fileSerial[v.name]%int(yxplog.maxFileCount) + 1)),"_",v.fileName)
	case 3:tp=fmt.Sprint(v.fileDir,v.fileName)
	default:tp=fmt.Sprint(v.fileDir,time.Now().Format(v.datePattern),v.fileName)
	}
	return tp
}

func checkFileDir() {
	for _,v:=range settingArr {
		tp:=v.fileDir
		//p, _ := path.Split(tp)
		d, err := os.Stat(tp)
		if err != nil || !d.IsDir() {
			if err := os.MkdirAll(tp, _FILE_CREAT_MODE_); err != nil {
				log.Println(_LABEL_, "create file fail!")
			}
		}
	}
}



//---------------------对外提供方法---------------
func Println(paramstr ...string)bool  {
	logstr:=""
	logType:=make([]string,0)
	for i,p:=range paramstr{
		if i==0 {
			logstr =fmt.Sprintf(
				"%s\t%d\t%s\n",
				time.Now().Format(TIMEFORMAT),
				"Println",
				p,
			)
		}else {
			logType=append(logType,p)
		}
	}
	if len(paramstr)==1{
		logType=append(logType,"Info")
	}
	doPrint(logstr,logType)
	return true
}

func InfoPrintln(paramstr string)bool  {
	logType:=make([]string,0)
	logType=append(logType,"Info")
	logstr :=fmt.Sprintf(
		"%s\t%d\n",
		time.Now().Format(TIMEFORMAT),
		paramstr,
	)
	doPrint(logstr,logType)
	return true
}

func DebugPrintln(paramstr string)bool  {
	logType:=make([]string,0)
	logType=append(logType,"Debug")
	logstr :=fmt.Sprintf(
		"%s\t%d\n",
		time.Now().Format(TIMEFORMAT),
		paramstr,
	)
	doPrint(logstr,logType)
	return true
}

func ErrorPrintln(paramstr string)bool  {
	logType:=make([]string,0)
	logType=append(logType,"Error")
	logstr :=fmt.Sprintf(
		"%s\t%d\n",
		time.Now().Format(TIMEFORMAT),
		paramstr,
	)
	doPrint(logstr,logType)
	return true
}
func doPrint(logstr string,logType []string)  {
	for _,v:=range logType {
		yxplog.yxpChannel[v]<-logstr
	}
}



