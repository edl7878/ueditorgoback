package ueditorgobackend

import (
	// "github.com/astaxie/beego" 1. uncomment this if you want to use it with beego
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type UEditorConfig map[string]interface{}
type UploadConfig struct {
	pathFormat string
	maxSize    int64
	allowFiles []string
	origName   string
}

var ueditorConfig UEditorConfig
var serverPath string
var configJsonPath = "../ueditor/go/config.json"

var stateMap = map[string]string{
	"0":                        "SUCCESS",
	"1":                        "文件大小超出 upload_max_filesize 限制",
	"2":                        "文件大小超出 MAX_FILE_SIZE 限制",
	"3":                        "文件未被完整上传",
	"4":                        "没有文件被上传",
	"5":                        "上传文件为空",
	"ERROR_TMP_FILE":           "临时文件错误",
	"ERROR_TMP_FILE_NOT_FOUND": "找不到临时文件",
	"ERROR_SIZE_EXCEED":        "文件大小超出网站限制",
	"ERROR_TYPE_NOT_ALLOWED":   "文件类型不允许",
	"ERROR_CREATE_DIR":         "目录创建失败",
	"ERROR_DIR_NOT_WRITEABLE":  "目录没有写权限",
	"ERROR_FILE_MOVE":          "文件保存时出错",
	"ERROR_FILE_NOT_FOUND":     "找不到上传文件",
	"ERROR_WRITE_CONTENT":      "写入文件内容错误",
	"ERROR_UNKNOWN":            "未知错误",
	"ERROR_DEAD_LINK":          "链接不可用",
	"ERROR_HTTP_LINK":          "链接不是http链接",
	"ERROR_HTTP_CONTENTTYPE":   "链接contentType不正确",
}

type Size interface {
	Size() int64
}

type Stat interface {
	Stat() (os.FileInfo, error)
}

func init() {
	serverPath = getCurrentPath()
	ueditorConfig = make(UEditorConfig)
	readConfig(configJsonPath)
}

/** 2. uncomment this if you want to use it with beego
type UEditorController struct{
	beego.Controller
}
**/

type Uploader struct {
	request   *http.Request
	fileField string
	file      multipart.File
	base64    string
	config    UEditorConfig
	oriName   string
	fileName  string
	fullName  string
	filePath  string
	fileSize  int64
	fileType  string
	stateInfo string
	optype    string
}

func HandleUpload(w http.ResponseWriter, r *http.Request) {
	var fieldName string = "upload"
	var config UEditorConfig
	var fbase64 string
	var result map[string]string
	var responseBytes []byte

	/*nginx解决了跨域，本片段注销
	//跨域上传，会首先发起OPTIONS请求，然后才是真实请求，这里进行处理
	if r.Method == "OPTIONS" {
		fmt.Println("Cors")
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers",
				"X_Requested_With,Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		}
		return
	}
	//真实请求从这里开始，若跨域，header仍需设置
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers",
			"X_Requested_With,Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
	}
	*/
	r.ParseMultipartForm(32 << 20) // TODO: read it from config

	//前端跨域请求会有callback字段，这里构造返回请求格式
	//同域正常返回，否则把返回值包到回调函数里作为语句返回
	//注意，任何回写response需要前缀语句：
	//resopnseTemplate[2]=string(responseBytes[:])
	//responseBytes=[]byte(strings.Join(resopnseTemplate,""))
	callBack, ok := r.Form["callback"]
	resopnseTemplate := []string{"FuncPlaceHolder", "(", "jsonPlaceHolder", ");"}
	if ok {
		w.Header().Set("Content-Type", "application/javascript")
		//fmt.Println(callBack[0])
		resopnseTemplate[0] = callBack[0]
	} else {
		resopnseTemplate[0] = ""
		resopnseTemplate[1] = ""
		resopnseTemplate[3] = ""
		w.Header().Set("Content-Type", "application/json;charset=utf8")
	}

	action := r.Form["action"]
	switch action[0] {
	case "config":
		responseBytes, _ = json.Marshal(&ueditorConfig)
		resopnseTemplate[2] = string(responseBytes[:])
		responseBytes = []byte(strings.Join(resopnseTemplate, ""))
		w.Write(responseBytes)
		return
	case "listimage":

		startstr, ok := r.Form["start"]
		if !ok {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求参数start出错"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		sizestr, ok := r.Form["size"]
		if !ok {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求参数size出错"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		start, err := strconv.Atoi(startstr[0])
		if err != nil {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求参数start出错"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		size, err := strconv.Atoi(sizestr[0])
		if err != nil {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求参数size出错"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		userID, ok := r.Form["userID"]
		if !ok {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求参数userID出错"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		pathFormat := ueditorConfig["imageManagerListPath"]
		allowFiles := ueditorConfig["imageManagerAllowFiles"]
		//读取目录下内容
		responseStruct := make(map[string]interface{})
		resFileList := make([](map[string]string), 0)
		pathFormatStr := pathFormat.(string)
		items, err := ioutil.ReadDir(pathFormatStr + "/" + userID[0])
		if err != nil {
			responseBytes, _ = json.Marshal(map[string]string{"state": "目录不存在"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		for _, item := range items {
			if !item.IsDir() { //如果不是目录
				name := item.Name()
				pos := strings.LastIndex(name, ".")
				extName := strings.ToLower(name[pos:])
				//按扩展名滤出需要的图片
				found := false
				for _, ext := range allowFiles.([]interface{}) {
					if ext.(string) == extName {
						found = true
						break
					}
				}
				if found == true { //符合要求文件
					resFileList = append(resFileList, map[string]string{"url": pathFormatStr + "/" + userID[0] + "/" + name})
				}
			}
		} //for end
		length := len(resFileList)
		if start < 0 || start >= length {
			responseBytes, _ = json.Marshal(map[string]string{"state": "请求范围越界"})
			resopnseTemplate[2] = string(responseBytes[:])
			responseBytes = []byte(strings.Join(resopnseTemplate, ""))
			w.Write(responseBytes)
			return
		}
		responseStruct["start"] = start
		if start+size-1 >= length {
			resFileList = resFileList[start:]
		} else {
			resFileList = resFileList[start : start+size] //不含start+size元素
		}
		responseStruct["list"] = resFileList
		responseStruct["total"] = length
		responseStruct["state"] = "SUCCESS"
		responseBytes, _ = json.Marshal(responseStruct)
		fmt.Println(string(responseBytes))
		resopnseTemplate[2] = string(responseBytes[:])
		responseBytes = []byte(strings.Join(resopnseTemplate, ""))
		w.Write(responseBytes)
		return

	case "uploadimage":
		config = UEditorConfig{
			"pathFormat": ueditorConfig["imagePathFormat"],
			"maxSize":    ueditorConfig["imageMaxSize"],
			"allowFiles": ueditorConfig["imageAllowFiles"],
		}
		fieldName = ueditorConfig["imageFieldName"].(string)

	case "uploadscrawl":
		config = UEditorConfig{
			"pathFormat": ueditorConfig["scrawPathFormat"],
			"maxSize":    ueditorConfig["scrawMaxSize"],
			"allowFiles": ueditorConfig["scrawlAllowFiles"],
			"oriname":    "scrawl.png",
		}
		fieldName = ueditorConfig["scrawFieldName"].(string)
		fbase64 = "base64"
	case "uploadvideo":
		config = UEditorConfig{
			"pathFormat": ueditorConfig["videoPathFormat"],
			"maxSize":    ueditorConfig["videoMaxSize"],
			"allowFiles": ueditorConfig["videoAllowFiles"],
		}
		fieldName = ueditorConfig["videoFieldName"].(string)
	case "uploadfile":
		config = UEditorConfig{
			"pathFormat": ueditorConfig["filePathFormat"],
			"maxSize":    ueditorConfig["fileMaxSize"],
			"allowFiles": ueditorConfig["fileAllowFiles"],
		}
		fieldName = ueditorConfig["fileFieldName"].(string)
	default:
		responseBytes, _ = json.Marshal(map[string]string{
			"state": "请求地址出错",
		})
		resopnseTemplate[2] = string(responseBytes[:])
		responseBytes = []byte(strings.Join(resopnseTemplate, ""))
		//fmt.Println("respenseBytes:", responseBytes)
		w.Write(responseBytes)
		return
	}
	config["maxSize"] = int64(config["maxSize"].(float64))
	uploader := NewUploader(r, fieldName, config, fbase64)

	userID, ok := r.Form["userID"]
	//支持UEditor serverparam，参数key定义为userID
	if ok {
		config["userID"] = userID[0]
	}

	uploader.upFile()
	result = uploader.getFileInfo()
	responseBytes, _ = json.Marshal(result)

	resopnseTemplate[2] = string(responseBytes[:])
	responseBytes = []byte(strings.Join(resopnseTemplate, ""))
	fmt.Println(string(responseBytes))
	w.Write(responseBytes)
}

/** 3. uncomment this if you want to use it with beego
func (this *UEditorController) Handle() {
	var fieldName string = "upload"
	var config UEditorConfig
	var fbase64 string
	var result map[string]string

	action := this.GetString("action")
	switch (action) {
	case "config":
		this.Data["json"] = &ueditorConfig
		this.ServeJSON()
		return
	case "uploadimage":
		config = UEditorConfig{
			"pathFormat" : ueditorConfig["imagePathFormat"],
			"maxSize" : ueditorConfig["imageMaxSize"],
			"allowFiles" : ueditorConfig["imageAllowFiles"],
		}
		fieldName = ueditorConfig["imageFieldName"].(string)
	case "uploadscrawl":
		config = UEditorConfig{
			"pathFormat" : ueditorConfig["scrawPathFormat"],
			"maxSize" : ueditorConfig["scrawMaxSize"],
			"allowFiles" : ueditorConfig["scrawlAllowFiles"],
			"oriname" : "scrawl.png",
		}
		fieldName = ueditorConfig["scrawFieldName"].(string)
		fbase64 = "base64"
	case "uploadvideo":
		config = UEditorConfig{
			"pathFormat" : ueditorConfig["videoPathFormat"],
			"maxSize": ueditorConfig["videoMaxSize"],
			"allowFiles" : ueditorConfig["videoAllowFiles"],
		}
		fieldName = ueditorConfig["videoFieldName"].(string)
	case "uploadfile":
		config = UEditorConfig{
			"pathFormat" : ueditorConfig["filePathFormat"],
			"maxSize" : ueditorConfig["fileMaxSize"],
			"allowFiles" : ueditorConfig["fileAllowFiles"],
		}
		fieldName = ueditorConfig["fileFieldName"].(string)
	default:
		this.Data["json"] = &map[string]string{
			"state" : "请求地址出错",
		}
		this.ServeJSON()
		return
	}
	config["maxSize"] = int64(config["maxSize"].(float64))
	uploader := NewUploader(this.Ctx.Request, fieldName, config, fbase64)

	uploader.upFile()
	result = uploader.getFileInfo()

	this.Data["json"] = &result
	this.ServeJSON()
}
**/
//新建并初始化Uploader
//fileField对应config.json中的fileFieldName
//config是UEditorConfig映射
//optype仅在uploader定义和本函数中出现，意义不详
func NewUploader(request *http.Request, fileField string, config UEditorConfig, optype string) (uploader *Uploader) {
	uploader = new(Uploader)
	uploader.request = request
	uploader.fileField = fileField
	uploader.config = config
	uploader.optype = optype

	return
}

//按照Upload的设置，上传文件
//并构建stateInfo返回信息
func (this *Uploader) upFile() {

	this.request.ParseMultipartForm(this.config["maxSize"].(int64))
	file, fheader, err := this.request.FormFile(this.fileField)
	defer file.Close()
	if err != nil {
		this.stateInfo = err.Error()
		fmt.Printf("upload file error: %s", err)
	} else { //如果parase form 成功
		this.oriName = fheader.Filename
		//支持Stat接口么？
		if stateInterface, ok := file.(Stat); ok {
			fileInfo, _ := stateInterface.Stat()
			this.fileSize = fileInfo.Size()
		} else if sizeInterface, ok := file.(Size); ok { //支持Size接口?
			this.fileSize = sizeInterface.Size()
		} else {//都不支持
			this.stateInfo = this.getStateInfo("ERROR_UNKNOWN")
			return
		}
		this.fileType = this.getFileExt()
		this.fullName = this.getFullName()
		this.filePath = this.getFilePath()
		this.fileName = this.getFileName()

		dirname := path.Dir(this.filePath)

		if !this.checkSize() {
			this.stateInfo = this.getStateInfo("ERROR_SIZE_EXCEED")
			return
		}

		if !this.checkType() {
			this.stateInfo = this.getStateInfo("ERROR_TYPE_NOT_ALLOWED")
			return
		}

		dirInfo, err := os.Stat(dirname)
		if err != nil {
			err = os.MkdirAll(dirname, 0666)
			if err != nil {
				this.stateInfo = this.getStateInfo("ERROR_CREATE_DIR")
				fmt.Printf("Error create dir: %s", err)
				return
			}
		} else if dirInfo.Mode()&0222 == 0 {
			this.stateInfo = this.getStateInfo("ERROR_DIR_NOT_WRITEABLE")
			return
		}

		fout, err := os.OpenFile(this.filePath, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			this.stateInfo = this.getStateInfo("ERROR_FILE_MOVE")
			return
		}
		defer fout.Close()

		io.Copy(fout, file)
		// if err != nil {
		// 	this.stateInfo = this.getStateInfo("ERROR_FILE_MOVE");
		// 	return
		// }

		this.stateInfo = stateMap["0"]
	}
}

//stateMap是包全局错误代码映射列表，可以根据错误代码映射一个错误信息
//例如："ERROR_TYPE_NOT_ALLOWED":   "文件类型不允许"
//用于返回客户端错误信息
func (this *Uploader) getStateInfo(errCode string) string {
	if errInfo, ok := stateMap[errCode]; ok {
		return errInfo
	} else {
		return stateMap["ERROR_UNKNOWN"]
	}
}

//取得Uploader.oriName的扩展名
func (this *Uploader) getFileExt() string {
	pos := strings.LastIndex(this.oriName, ".")
	return strings.ToLower(this.oriName[pos:])
}

//按照pathFormat定义，产生一个路径文件名
//在handleUplad函数中，该pathFormat被赋值为config.json的对应路径设置
//例如：
// case "uploadimage":
// 	config = UEditorConfig{
// 		"pathFormat": ueditorConfig["imagePathFormat"],
// 		"maxSize":    ueditorConfig["imageMaxSize"],
// 		"allowFiles": ueditorConfig["imageAllowFiles"],
// 	}
func (this *Uploader) getFullName() string {
	t := time.Now()
	format := this.config["pathFormat"].(string)
	//这里支持UEditor服务器参数：serverparam
	//规定：execCommand('serverparam', 'userID', '...')
	//"imagePathFormat": "upload/image/{yyyy}{mm}{dd}{time}{rand:6}"
	//{yyyy}{mm}{dd}{time}{rand:6}部分应不出现"/"符号，否则listimage Action无法解析
	index := strings.LastIndex(format, "/")
	if index != -1 {
		userID, ok := this.config["userID"]
		if ok {
			format = format[0:index+1] + userID.(string) + "/" + format[index:]
		}
	}
	format = strings.Replace(format, "{yyyy}", strconv.Itoa(t.Year()), 1)

	format = strings.Replace(format, "{mm}", strconv.Itoa(int(t.Month())), 1)
	format = strings.Replace(format, "{dd}", strconv.Itoa(t.Day()), 1)
	format = strings.Replace(format, "{hh}", strconv.Itoa(t.Hour()), 1)
	format = strings.Replace(format, "{ii}", strconv.Itoa(t.Minute()), 1)
	format = strings.Replace(format, "{ss}", strconv.Itoa(t.Second()), 1)
	format = strings.Replace(format, "{time}", strconv.FormatInt(t.Unix(), 10), 1)

	reg := regexp.MustCompile("{rand:[0-9]+}")
	randstrs := reg.FindAllString(format, -1)

	randNum := ""
	if len(randstrs) != 0 {
		//只考虑第一个{rand:n}
		reg = regexp.MustCompile("[0-9]+")
		digitNumber, err := strconv.Atoi(reg.FindAllString(randstrs[0], -1)[0])
		if err == nil {
			for i := 1; i <= digitNumber; i++ {
				randNum += strconv.Itoa(rand.Intn(10))
			}

			format = strings.Replace(format, randstrs[0], randNum, 1)
		}
	}
	//format = format + randNum

	return format + this.getFileExt()
}

//返回Uploader.filePath路径文件名中的文件名
func (this *Uploader) getFileName() string {
	pos := strings.LastIndex(this.filePath, "/")
	return this.filePath[pos+1:]
}

//从Upload.fullName：pathformat+文件名
//getFilePath:serverpath+fullname
func (this *Uploader) getFilePath() string {
	fullname := this.fullName
	//原来的代码，fullname最后一个字符是"/",前面加个"/"
	//无用逻辑
	if strings.LastIndex(fullname, "/") == (len(fullname) - 1) {
		fullname = "/" + fullname
	}
	//如果fullname开头没有"/",加一个
	if !strings.HasPrefix(fullname, "/") {
		fullname = "/" + fullname
	}
	return serverPath + fullname
}

//检查文件扩展名是否在允许列表中
func (this *Uploader) checkType() bool {
	found := false
	for _, ext := range this.config["allowFiles"].([]interface{}) {
		if ext.(string) == this.fileType {
			found = true
			break
		}
	}

	return found
}

//文件小于指定大小，返回true
func (this *Uploader) checkSize() bool {
	return this.fileSize <= (this.config["maxSize"].(int64))
}

//构建返回给客户端的信息
func (this *Uploader) getFileInfo() map[string]string {
	return map[string]string{
		"state":    this.stateInfo,
		"url":      this.fullName,
		"title":    this.fileName,
		"original": this.oriName,
		"type":     this.fileType,
		"size":     string(this.fileSize),
	}
}

//将指定位置的config文件映射到ueditorconfig结构里
//serverpath在模块初始化时被赋值为当前路径
func readConfig(configPath string) {
	fd, err := os.Open(serverPath + "/" + configPath)
	checkError(err)
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	checkError(err)

	pattern := regexp.MustCompile(`/\*.+?\*/`)
	data = pattern.ReplaceAll(data, []byte(""))

	json.Unmarshal(data, &ueditorConfig)
	//fmt.Println(string(data))
}

//取得当前路径:exe文件所在路径
func getCurrentPath() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	checkError(err)
	return strings.Replace(dir, "\\", "/", -1)
}

//输出错误信息
func checkError(err error) {
	if err != nil {
		fmt.Printf("Error: %s", err)
	}
}
