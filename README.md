# m3u8_downloader

> 一个简单的m3u8下载工具

## 命令参数

```shell
# go run main.go -u https://www.baidu.com/index.m3u8 --output_path ./downloads --name baidu.mp4
-h 查看帮助
--g_num 设置下载ts的最大goroutine数,默认为cpu数量的5倍
--name 输出文件的名字
--out_path 设置下载文件的保存路径,默认./downloads
-u 设置m3u8的url
```