package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

const filePath = "./url-shortener.txt"

var (
	file          *os.File
	urls          []string
	savedUrlCount int
	mutex         sync.RWMutex
)

// custom base64 order: - _ 0-9 a-z A-Z
// absolute shit speed conversion functions
// apparently it's faster to use array lookup like
// var base64ToByteTable []byte
// var byteToBase64Table []byte
// and even faster to use simd. I aint doing that though
func base64StringToIdx(bytes []byte) (idx int) {
	if len(bytes) > 4 {
		return -1
	}
	idx = 0
	for i := 0; i < len(bytes); i++ {
		currentByte := bytes[i]
		if currentByte == 0x2d {
			currentByte = 0
		} else if currentByte == 0x5f {
			currentByte = 1
		} else if 0x30 <= currentByte && currentByte <= 0x39 {
			currentByte -= 46
		} else if 0x61 <= currentByte && currentByte <= 0x7A {
			currentByte -= 97 - 12
		} else if 0x41 <= currentByte && currentByte <= 0x5A {
			currentByte -= 65 - 38
		} else {
			return -1
		}

		idx |= int(currentByte) << (i * 6)
	}
	return
}

func idxToBase64String(idx int32) (str string) {
	if idx == 0 {
		return "-"
	}
	str = ""
	for idx > 0 {
		part := idx & 0x3f
		if part == 0 {
			str += "-"
		} else if part == 1 {
			str += "_"
		} else if part <= 11 {
			str += string(byte(part + 46))
		} else if part <= 37 {
			str += string(byte(part + 85))
		} else {
			str += string(byte(part + 27))
		}
		idx >>= 6
	}
	return
}

func breakOn(e error) {
	if e != nil {
		panic(e)
	}
}

func rootHandler(ctx *fasthttp.RequestCtx) {
	pathBytes := ctx.Request.Header.RequestURI()
	// using this instead of ctx.Path() because path is 'normalized' path, which removes .. and collapses multiple consecutive slashes //.

	// code to show all the urls. only for development
	// if len(pathBytes) >= 17 && string(pathBytes)[:17] == "/showmealltheurls" {
	// 	ctx.WriteString(fmt.Sprintf("%v", urls))
	// } else

	if len(pathBytes) > 9 && string(pathBytes)[:9] == "/add-url/" {
		path := string(pathBytes)
		url := path[9:] // length of /add-url/
		mutex.Lock()
		idx := len(urls)
		urls = append(urls, url)
		mutex.Unlock()

		ctx.WriteString("your url is " + idxToBase64String(int32(idx)))
	} else {
		reqHex := pathBytes[1:]
		index := base64StringToIdx(reqHex)
		mutex.RLock()
		if index < 0 || index >= len(urls) {
			ctx.WriteString("Invalid URL")
			return
		}
		url := urls[index]
		mutex.RUnlock()
		ctx.Response.Header.Set("Location", url)
		ctx.SetStatusCode(301)
	}
}

func saveUrls() {
	mutex.RLock()
	defer mutex.RUnlock()
	if len(urls) > savedUrlCount {
		oldSavedUrlCount := savedUrlCount
		savedUrlCount = len(urls)
		fmt.Printf("writing %v urls\n", savedUrlCount-oldSavedUrlCount)
		_, err2 := file.WriteString(strings.Join(urls[oldSavedUrlCount:savedUrlCount], "\n") + "\n")
		breakOn(err2)
	}
}

func main() {
	fmt.Println("starting url shortener")
	dat, err := ioutil.ReadFile(filePath)
	if err != nil || len(dat) < 1 {
		ioutil.WriteFile(filePath, []byte(""), 0644)
	} else {
		filestring := string(dat)
		urls = strings.Split(filestring, "\n")
		urls = urls[:len(urls)-1]
		// each line is appended with a newline, so we have to remove the trailing newline
	}
	savedUrlCount = len(urls)

	file, err = os.OpenFile(filePath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644)
	breakOn(err)

	ticker := time.NewTicker(5 * time.Millisecond)

	go func() {
		for {
			<-ticker.C
			saveUrls()
		}
	}()
	println("ready to accept requests")

	// Start service
	fasthttp.ListenAndServe("127.0.0.1:8090", rootHandler)
}
