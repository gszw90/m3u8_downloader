package m3u8

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

var client *http.Client

func init() {
	// 跳过证书验证
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client = &http.Client{Transport: transport}

}

// M3U8 参数: https://blog.csdn.net/weixin_41635750/article/details/108066684
type M3U8 struct {
	IsM3U8 bool
	// 上级m3u8的url
	ParentUrl string
	// url
	Url string
	// 主域名
	BaseUrl string
	// m3u8版本
	Version int
	// 最大媒体段时长(秒)
	TargetDuration int
	// 首个list的序号
	MediaSequence int
	// ts列表
	//TsLists []string
	TsLists []Ts
	// 是否需要解密
	HasKey bool
	// 加密key
	Keys []Key
	// 播放列表,有带宽,分辨率等信息,用户根据带宽自行选择播放文件
	StreamInfos []StreamInfo

	// 保存的内容
	OutputName string
	// 保存目录
	OutputPath string
}

type Key struct {
	// 加密方法,如果是NONE标识不需要解密,AES-128使用aes128解密
	Method string
	// 秘钥地址
	Uri string
	// key的值
	KeyValue []byte
	// 加密向量
	IV string
}

// StreamInfo 多带宽信息
type StreamInfo struct {
	// 带宽
	BandWidth int
	// 标识,一个playlist中可能有多个相同的标识
	ProgramId int
	// 文件格式列表
	Codecs string
	// 分辨率
	Resolution string
	Audio      string
	Video      string
	// 播放列表的m3u8信息
	SubM3u8 M3U8
}

// Ts ts文件
type Ts struct {
	Index int    // 文件顺序
	Url   string // 文件的url
	Dist  string // 文件的下载保存地址
	Key   Key    // 如果文件被加密,加密的信息
}

func NewM3U8(url string) (m3u8 *M3U8, err error) {
	m3u8 = &M3U8{}
	err = m3u8.ParseUrl(url)
	if err != nil {
		return
	}
	// 如果存在多个playlist,获取带宽最大的作为下载对象
	if len(m3u8.TsLists) == 0 && len(m3u8.StreamInfos) > 0 {
		streamInfo := m3u8.GetMaxBandwidthInfo()
		parentUrl := m3u8.Url
		m3u8 = &streamInfo.SubM3u8
		m3u8.ParentUrl = parentUrl
		err = m3u8.ParseUrl(streamInfo.SubM3u8.Url)
		if err != nil {
			return nil, err
		}
	}
	return
}

// SetOutputPath 设置输出路劲
func (m *M3U8) SetOutputPath(name string) {
	m.OutputPath = name
}

// SetOutputName 甚至输出文件名
func (m *M3U8) SetOutputName(name string) {
	m.OutputName = name
}

// Run 启动下载解析任务
func (m *M3U8) Run() (err error) {
	err = m.DownloadTsList()
	if err != nil {
		m.CleanTsLists()
		return err
	}
	err = m.MergeTsList()
	if err != nil {
		m.CleanTsLists()
		os.Remove(m.OutputPath + "/" + m.OutputName)
		return err
	}
	log.Printf("output file:%s/%s\n", m.OutputPath, m.OutputName)
	return
}

// DownloadTsList 下载ts列表
func (m *M3U8) DownloadTsList() (err error) {
	fmt.Printf("start download ts files from:%s\n", m.Url)
	if !m.IsM3U8 {
		err = errors.New("非m3u8格式文件")
		return err
	}
	//if m.HasKey {
	//	err = errors.New("暂不支持加密数据")
	//	return err
	//}
	tsLength := len(m.TsLists)
	if tsLength == 0 {
		err = errors.New("ts数据列表为空")
		return err
	}
	if m.OutputPath == "" {
		m.OutputPath = "./downloads"
	}
	// 创建保存目录
	_, err = isFileExisted(m.OutputPath, true)
	if err != nil {
		return err
	}
	//_, err = isFileExisted(m.OutputPath+"/ts", true)
	//if err != nil {
	//	return err
	//}
	goroutineNum := 16
	downloadChan := make(chan Ts, goroutineNum)
	doneChan := make(chan int)
	finished := false
	// ts文件的总数量
	var tsNum = int32(len(m.TsLists))
	// 已处理ts文件的数量
	var finishedNum int32
	// 已结束工作的goroutine的数量
	var finishedGoroutineNum int32
	// 分发ts下载任务
	go func() {
		for k, v := range m.TsLists {
			v.Dist = fmt.Sprintf("%s/%s_%05d.ts", m.OutputPath, m.OutputName, k)
			v.Index = k
			m.TsLists[k].Dist = v.Dist
			downloadChan <- v
		}
	}()
	var downloadErr error
	// 在goroutine中下载ts文件
	for i := 0; i < goroutineNum; i++ {
		go func() {
			for {
				select {
				case ts := <-downloadChan:
					if finished {
						goto finished
					}
					err := DownloadTs(ts)
					atomic.AddInt32(&finishedNum, 1)
					if err != nil {
						downloadErr = err
						finished = true
						goto finished
					}
				default:
					//fmt.Printf("go default:%d,ts num:%d\n",atomic.LoadInt32(&finishedNum),tsNum)
					if atomic.LoadInt32(&finishedNum) == tsNum {
						fmt.Printf("stop one goroutine\n")
						goto finished
					}
				}
			}
		finished:
			n := atomic.AddInt32(&finishedGoroutineNum, 1)
			fmt.Printf("stop goroutine:%d\n", n)
			if n == int32(goroutineNum) {
				doneChan <- 1
			}
		}()
	}
	//wg.Wait()
	<-doneChan
	err = downloadErr
	return
}

// MergeTsList 合并ts文件
func (m *M3U8) MergeTsList() (err error) {
	//log.Printf("start to merge ts files as:%s/%s\n", m.OutputPath, m.OutputName)
	if len(m.TsLists) == 0 {
		err = errors.New("ts list is empty")
		return err
	}
	// 创建保存文件
	f, err := os.Create(m.OutputPath + "/" + m.OutputName)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	for k, ts := range m.TsLists {
		// 检查ts文件是否存在
		//tsFile := fmt.Sprintf("%s/ts/%s_%05d.ts", m.OutputPath, m.OutputName, i)
		//fmt.Printf("merge file:%s\n",ts.Dist)
		isExited, _ := isFileExisted(ts.Dist, false)
		if !isExited {
			err = fmt.Errorf("ts file[%s] is not exited", ts.Dist)
			return err
		}
		// 读取ts文件数据,并写入目标文件中
		tsContent, err := ioutil.ReadFile(ts.Dist)
		if err != nil {
			return err
		}
		_, err = writer.Write(tsContent)
		if err != nil {
			return err
		}
		// 每十个ts文件合并一次
		if (k+1)%10 == 0 {
			err = writer.Flush()
			if err != nil {
				return err
			}
		}
	}
	err = writer.Flush()
	m.CleanTsLists()
	return err
}

// CleanTsLists 清理ts文件
func (m *M3U8) CleanTsLists() (err error) {
	for _, ts := range m.TsLists {
		if ts.Dist == "" {
			continue
		}
		isExisted, _ := isFileExisted(ts.Dist, false)
		if !isExisted {
			continue
		}
		err = os.Remove(ts.Dist)
		if err != nil {
			return err
		}
	}
	//err = os.RemoveAll(tsDir)
	return err
}

// DownloadTs 下载单个ts
func DownloadTs(ts Ts) (err error) {
	//log.Printf("start download ts from:%s\n", ts.Url)
	//log.Printf("start download ts to:%s\n", ts.Dist)
	// 重试次数
	tryTimes := 3
	var tsContent []byte
	for ; tryTimes > 0; tryTimes-- {
		tsContent, err = getRequest(ts.Url)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if ts.Key.Uri != "" && ts.Key.Method != "NONE" {
		// 获取加密文件的key
		keyBytes, err := getRequest(ts.Key.Uri)
		if err != nil {
			return err
		}
		ts.Key.KeyValue = keyBytes
		fmt.Printf("key => %+v\n", ts.Key)
		// 解密ts文件
		tsContent, err = AesDecrypt(tsContent, keyBytes, []byte(ts.Key.IV))
		if err != nil {
			return err
		}
	}
	// 下载内容写入文件
	if err != nil {
		return err
	}
	// 某些ts文件并非以0x47开头,合并后文件无法播放,需要移除0x47前面的数据
	syncByte := uint8(71) //0x47
	bLen := len(tsContent)
	for j := 0; j < bLen; j++ {
		if tsContent[j] == syncByte {
			tsContent = tsContent[j:]
			break
		}
	}
	err = ioutil.WriteFile(ts.Dist, tsContent, os.ModePerm)
	return
}

// ParseUrl 解析m3u8 地址
func (m *M3U8) ParseUrl(m3u8Url string) (err error) {
	uri, err := url.Parse(m3u8Url)
	if err != nil {
		return
	}
	baseUrl := uri.Scheme + "://" + uri.Host
	m.BaseUrl = baseUrl
	m.Url = m3u8Url
	m.TsLists = []Ts{}
	m.Keys = []Key{}

	req, err := http.NewRequest(http.MethodGet, m3u8Url, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// 解析内容
	scanner := bufio.NewScanner(resp.Body)
	// 播放列表信息
	var streamItem StreamInfo
	var keyItem Key
	var hasPreKey bool
	var preIsPlaylist bool
	// 逐行解析m3u8文件信息
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXTINF:"):
			continue
		case line == "#EXTM3U":
			m.IsM3U8 = true
		case strings.HasPrefix(line, "#EXT-X-VERSION:"):
			if _, err := fmt.Sscanf(line, "#EXT-X-VERSION:%d", &(m.Version)); err != nil {
				return err
			}
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			if _, err := fmt.Sscanf(line, "#EXT-X-TARGETDURATION:%d", &(m.TargetDuration)); err != nil {
				return err
			}
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			if _, err := fmt.Sscanf(line, "#EXT-X-MEDIA-SEQUENCE:%d", &(m.TargetDuration)); err != nil {
				return err
			}
		case strings.HasPrefix(line, "#EXT-X-KEY"):
			keyItem = Key{}
			// method
			regExp := `METHOD=(?P<method>(NONE|AES-128))`
			methodStr := getOneByReg(regExp, line)
			keyItem.Method = methodStr
			// uri
			regExp = `URI="(?P<url>.+)"`
			uriStr := getOneByReg(regExp, line)
			keyItem.Uri = uriStr
			// IV
			regExp = `IV=(?P<resolution>[0-9a-zA-Z]+)`
			ivStr := getOneByReg(regExp, line)
			keyItem.IV = ivStr
			m.HasKey = true
			hasPreKey = true
			//m.Keys = append(m.Keys,key)
		case strings.HasSuffix(line, ".ts"), strings.Contains(line, ".ts?"):
			// 获取ts文件信息
			line = m.getHostUrl(line)
			ts := Ts{
				Index: 0,
				Url:   line,
				Dist:  "",
				Key:   Key{},
			}
			if hasPreKey {
				ts.Key = keyItem
				hasPreKey = false
			}
			m.TsLists = append(m.TsLists, ts)
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			// 获取多带宽信息
			streamItem = StreamInfo{}
			//带宽
			regExp := `BANDWIDTH=(?P<bandwidth>\d+)`
			bandWidthStr := getOneByReg(regExp, line)
			if bandWidthStr == "" {
				err = errors.New("can not get bandwidth")
				return err
			}
			bandwidth, err := strconv.Atoi(bandWidthStr)
			if err != nil {
				err = errors.New("wrong bandwidth value")
				return err
			}
			streamItem.BandWidth = bandwidth
			//programId
			regExp = `PROGRAM-ID=(?P<program>\d+)`
			programIdStr := getOneByReg(regExp, line)
			if programIdStr != "" {
				programId, err := strconv.Atoi(programIdStr)
				if err == nil {
					streamItem.ProgramId = programId
				}
			}
			// CODECS
			regExp = `CODECS="(?P<code>.+)"`
			codescStr := getOneByReg(regExp, line)
			streamItem.Codecs = codescStr
			// RESOLUTION
			regExp = `RESOLUTION=(?P<resolution>(\d+)x(\d+))`
			resolutionStr := getOneByReg(regExp, line)
			streamItem.Resolution = resolutionStr
			preIsPlaylist = true

		case strings.HasSuffix(line, ".m3u8"):
			// 多带宽对应的m3u8文件信息
			if preIsPlaylist {
				// playlist 下面的m3u8地址
				subM3u8 := m.getHostUrl(line)
				streamItem.SubM3u8 = M3U8{
					IsM3U8:      false,
					Url:         subM3u8,
					BaseUrl:     m.BaseUrl,
					TsLists:     []Ts{},
					StreamInfos: []StreamInfo{},
					OutputName:  m.OutputName,
					OutputPath:  m.OutputPath,
				}
				m.StreamInfos = append(m.StreamInfos, streamItem)
				preIsPlaylist = false
			}
		default:
			continue
		}
	}
	return
}

// GetMaxBandwidthInfo 获取最大带宽的播放列表信息
func (m *M3U8) GetMaxBandwidthInfo() (maxInfo *StreamInfo) {
	maxBandWidth := 0
	var index int
	for k, v := range m.StreamInfos {
		k := k
		if v.BandWidth > maxBandWidth {
			maxBandWidth = v.BandWidth
			index = k
		}
	}
	return &m.StreamInfos[index]
}

// getTsUrl 获取ts的url
func (m *M3U8) getHostUrl(tsUrl string) (newTsUrl string) {
	if strings.HasPrefix(tsUrl, "http") {
		newTsUrl = tsUrl
		return
	}
	if strings.HasPrefix(tsUrl, "/") {
		newTsUrl = m.BaseUrl + tsUrl
		return
	}
	// ts文件与m3u8文件在同一个目录下
	baseName := path.Base(m.Url)
	newTsUrl = strings.Replace(m.Url, baseName, tsUrl, 1)
	return
}

// 检查文件目录是否存在
func isFileExisted(filePath string, isCreate bool) (exited bool, err error) {
	_, e := os.Stat(filePath)
	if e == nil || os.IsExist(e) {
		exited = true
		return
	}
	// 文件不存在,则创建
	if isCreate {
		e = os.MkdirAll(filePath, os.ModePerm)
		if e == nil {
			err = nil
			exited = true
		} else {
			err = e
		}
	}
	return
}

// 从字符串中匹配第一个值
func getOneByReg(regExp string, str string) string {
	//str := "#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=68900,CODECS=\"mp4a.40.2\",SOLUTION=680x270"
	reg := regexp.MustCompile(regExp)
	reg.MatchString(str)
	res := reg.FindStringSubmatch(str)
	if len(res) >= 2 {
		return res[1]
	}
	return ""
}

// getRequest 请求url
func getRequest(url string) (content []byte, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	content, err = ioutil.ReadAll(resp.Body)
	return
}
