package main

import (
	m3u82 "github.com/XXzengweiXX/m3u8_downloader/m3u8"
	"github.com/spf13/cobra"
	"log"
	"runtime"
)

var m3u8Cmd = &cobra.Command{
	Use:   "m3u8",
	Short: "m3u8",
	Long:  `m3u8 downloader`,
	Run: func(cmd *cobra.Command, args []string) {
		run()
	},
}
var M3u8Url string
var OutputPath string
var OutputName string
var MaxGoroutine int

func init() {
	m3u8Cmd.Flags().StringVarP(&M3u8Url, "url", "u", "", "m3u8 url")
	m3u8Cmd.Flags().StringVar(&OutputPath, "output_path", "./downloads", "save path")
	m3u8Cmd.Flags().StringVar(&OutputName, "name", "", "file name")
	m3u8Cmd.Flags().IntVar(&MaxGoroutine, "g_num", runtime.NumCPU()*5, "the number of goroutines")
}

func main() {
	err := m3u8Cmd.Execute()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func run() {
	//urlString := "https://v2.xw0371.com/20180401/wiyCDyE3/index.m3u8"
	// normal
	//urlString := "https://shangzhibo-img.b0.upaiyun.com/client/user/100994/1526289188264/1526289188242_Session1GANandSynthesis-processed.m3u8"
	// bandWidth
	// http://vod.ijntv.cn/2016/1477/6417/1115/147764171115.ssm/147764171115.m3u8
	// http://vod.ijntv.cn/2016/1477/6417/1115/147764171115.ssm/147764171115-419k.m3u8
	// key
	// https://1252524126.vod2.myqcloud.com/9764a7a5vodtransgzp1252524126/0176cbbd5285890799673243539/drm/v.f230.m3u8
	if M3u8Url == "" {
		log.Println("url is empty")
		return
		//M3u8Url = "https://1252524126.vod2.myqcloud.com/9764a7a5vodtransgzp1252524126/0176cbbd5285890799673243539/drm/v.f230.m3u8"
	}
	if OutputName == "" {
		log.Println("file name is empty")
		return
		//OutputName = "output_key.mp4"
	}

	m3u8, err := m3u82.NewM3U8(M3u8Url)
	if err != nil {
		log.Fatal(err.Error())
	}
	m3u8.SetOutputPath(OutputPath)
	m3u8.SetOutputName(OutputName)
	m3u8.SetGNum(MaxGoroutine)
	// 运行
	err = m3u8.Run()
	if err != nil {
		log.Println(err.Error())
	}
	log.Println("finished")
}
