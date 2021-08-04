package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

type Downloader struct {
	concurrentNum int
}

func NewDownloader(concurrentNum int) *Downloader {
	return &Downloader{concurrentNum: concurrentNum}
}

func (d *Downloader) Download(strUrl, filename string) error {
	if filename == "" {
		filename = path.Base(strUrl)
	}
	resp, err := http.Head(strUrl)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK && resp.Header.Get("Accept-Ranges") == "bytes" {
		return d.multiDownload(strUrl, filename, resp.ContentLength)
	} else {
		return d.singleDownload(strUrl, filename)
	}
}

func (d *Downloader) multiDownload(url string, filename string, contentLen int64) error {
	partSize := contentLen / int64(d.concurrentNum)

	// 创建部分文件的存放目录
	partDir := d.getPartDir(filename)
	_ = os.Mkdir(partDir, 0777)
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(partDir)

	var wg sync.WaitGroup
	wg.Add(d.concurrentNum)

	var rangeStart int64
	for i := 0; i < d.concurrentNum; i++ {
		go func(i int, rangeStart int64) {
			wg.Done()
			rangeEnd := rangeStart + partSize
			if rangeEnd >= contentLen {
				rangeEnd = contentLen
			}
			d.downloadPartial(url, filename, rangeStart, rangeEnd, i)
		}(i, rangeStart)

		rangeStart += partSize + 1
	}
	wg.Wait()
	//合并文件
	err := d.mergeFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func (d *Downloader) singleDownload(url string, filename string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	flags := os.O_CREATE | os.O_WRONLY
	destFile, err := os.OpenFile(filename, flags, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer func(destFile *os.File) {
		_ = destFile.Close()
	}(destFile)

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(destFile, resp.Body, buf)
	if err != nil {
		if err == io.EOF {
			return nil
		}
		log.Fatal(err)
	}
	return nil
}

//downloadPartial
//@Description: 分片下载
func (d *Downloader) downloadPartial(url string, filename string, rangeStart, rangeEnd int64, i int) {
	if rangeStart > rangeEnd {
		return
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	flags := os.O_CREATE | os.O_WRONLY
	partFile, err := os.OpenFile(d.getPartFilename(filename, i), flags, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer func(partFile *os.File) {
		_ = partFile.Close()
	}(partFile)

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(partFile, resp.Body, buf)
	if err != nil {
		if err == io.EOF {
			return
		}
		log.Fatal(err)
	}
}

// getPartDir 部分文件存放的目录
func (d *Downloader) getPartDir(filename string) string {
	return strings.SplitN(filename, ".", 2)[0]
}

// getPartFilename 构造部分文件的名字
func (d *Downloader) getPartFilename(filename string, partNum int) string {
	partDir := d.getPartDir(filename)
	return fmt.Sprintf("%s/%s-%d", partDir, filename, partNum)
}

func (d *Downloader) mergeFile(filename string) error {
	destFile, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer func(destFile *os.File) {
		_ = destFile.Close()
	}(destFile)

	for i := 0; i < d.concurrentNum; i++ {
		partFilename := d.getPartFilename(filename, i)
		partFile, _ := os.Open(partFilename)
		_, _ = io.Copy(destFile, partFile)
		_ = partFile.Close()
		_ = os.Remove(partFilename)
	}
	return nil
}
