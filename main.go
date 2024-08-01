package main

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"gopkg.in/ini.v1"
)

func main() {

	cfg := readINIFile()

	//ip := "127.0.0.1"                                                        //ip address server
	//port := "80"                                                             // port server
	//grMax := 128                                                             //максимальное количество потоков (до 1024)
	//needFiles := []string{".hide.txt", "$MFT", "ntuser.dat"}                 //подстроки которые надо искать в названиях файла
	//needHashPath := []string{"\\Users\\mars\\Desktop\\develop\\mftwithname"} //папки в которых надо подсчитать хэш
	//needHash := []string{"f726d50e8bf94b6747ea9b07236851eb"}                 //массив искомых хэшей
	//onlyFind := true                                                         //индикатор обозначающий вывод всех хэшей в папках поиска
	//advanceHashInNeedFiles := false                                          //индикатор подсчета хэша для найденных по фильтру needFiles строк
	hostname, err := os.Hostname()                            // название машины
	logFileName := fmt.Sprintf("%s_checkerKUL.log", hostname) // название log-file

	if err != nil {
		fmt.Println("Не удалось выяснить имя хоста!")
		panic(err)
	}

	f, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)
	log.Printf("Старт:")

	// Root directory to start the walk
	root := "/"

	var wg sync.WaitGroup
	ch := make(chan int, cfg.grMax)

	// Slice to hold the file paths
	var files []string

	// Function to be called for each file or directory found
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Handle the error and continue walking
			fmt.Println(err)
			return nil
		}
		// Check if the path is a file
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	log.Printf("Совпадения по имени:")
	//ищем файлы по подстроке
	for _, sFile := range cfg.needFiles {
		for _, file := range files {
			matched, _ := regexp.MatchString(sFile, file)
			if matched {
				//if strings.Contains(strings.ToLower(file), strings.ToLower(sFile)) {
				//if strings.Contains(strings.ToLower(file), strings.ToLower(sFile)) {
				go func() {
					ch <- 1
					wg.Add(1)
					defer func() { wg.Done(); <-ch }()
					//если указано что надо подсчиттать хэш для найденных файлов
					if cfg.advanceHashInNeedFiles {
						MD5Sum, errMD5 := getHashMD5(file)
						SHA1Sum, errSHA1 := getHashSHA1(file)
						if errMD5 != nil {
							fmt.Printf("Ошибка получения md5 в файле %s", file)
							return
						}
						if errSHA1 != nil {
							fmt.Printf("Ошибка получения sha1 в файле %s", file)
							return
						}
						log.Printf("%s				::		%s", MD5Sum, file)
						log.Printf("%s		::		%s", SHA1Sum, file)
					} else {
						log.Printf("%s", file)
					}
					return
				}()
			}
		}
	}
	wg.Wait()
	log.Printf("Подсчет хэшей в директории:")
	fmt.Printf("Подсчет хэшей в директории:")
	//ищем хэши файлов
	for _, sFile := range cfg.needHashPath {
		for _, file := range files {
			matched, _ := regexp.MatchString(sFile, file)
			if matched {
				//fmt.Println(file)
				go func() {
					ch <- 1
					wg.Add(1)
					defer func() { wg.Done(); <-ch }()
					MD5Sum, errMD5 := getHashMD5(file)
					SHA1Sum, errSHA1 := getHashSHA1(file)
					if errMD5 != nil {
						fmt.Printf("Ошибка получения md5 в файле %s", file)
						//continue
						return
					}
					if errSHA1 != nil {
						fmt.Printf("Ошибка получения sha1 в файле %s", file)
						//continue
						return
					}
					SHA1Sum = strings.ToLower(SHA1Sum)
					MD5Sum = strings.ToLower(MD5Sum)
					//если необходимо осуществить поиск по хэшу
					if cfg.onlyFind {
						//если найден хэшу
						if slices.Contains(cfg.needHash, MD5Sum) || slices.Contains(cfg.needHash, SHA1Sum) {
							log.Printf("%s			::		%s", MD5Sum, file)
							log.Printf("%s	::		%s", SHA1Sum, file)
						}
					} else {
						log.Printf("%s			::		%s", MD5Sum, file)
						log.Printf("%s	::		%s", SHA1Sum, file)
					}
					return
				}()
			}
		}
	}
	wg.Wait()
	// Check for any errors from the Walk function
	if err != nil {
		fmt.Printf("error walking the path %q: %v\n", root, err)
		return
	}

	_, err = uploadFileMultipart(logFileName, cfg.ip, cfg.port)
	if err != nil {
		fmt.Println("Удачно отправлено!")
	}
	fmt.Println("Не отправлено!")
}

func getHashMD5(filePath string) (string, error) {
	h := md5.New()
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		fmt.Println(err)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func getHashSHA1(filePath string) (string, error) {
	h := sha1.New()
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		fmt.Println(err)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uploadFileMultipart(path string, ip string, port string) (*http.Response, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}

	// Reduce number of syscalls when reading from disk.
	bufferedFileReader := bufio.NewReader(f)
	defer f.Close()

	// Create a pipe for writing from the file and reading to
	// the request concurrently.
	bodyReader, bodyWriter := io.Pipe()
	formWriter := multipart.NewWriter(bodyWriter)

	// Store the first write error in writeErr.
	var (
		writeErr error
		errOnce  sync.Once
	)
	setErr := func(err error) {
		if err != nil {
			errOnce.Do(func() { writeErr = err })
		}
	}
	go func() {
		partWriter, err := formWriter.CreateFormFile("file", path)
		setErr(err)
		_, err = io.Copy(partWriter, bufferedFileReader)
		setErr(err)
		setErr(formWriter.Close())
		setErr(bodyWriter.Close())
	}()

	urlString := fmt.Sprintf("http://%s:%s/%s", ip, port, "upload")
	req, err := http.NewRequest(http.MethodPost, urlString, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", formWriter.FormDataContentType())

	// This operation will block until both the formWriter
	// and bodyWriter have been closed by the goroutine,
	// or in the event of a HTTP error.
	resp, err := http.DefaultClient.Do(req)

	if writeErr != nil {
		return nil, writeErr
	}

	return resp, err
}

func readINIFile() Kuls {

	conf := Kuls{}
	inidata, err := ini.Load("kaef.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}
	section := inidata.Section("kaef")

	conf.ip = section.Key("ip").String()
	conf.port = section.Key("port").String()
	conf.grMax = 128
	buff := section.Key("needFiles").String()
	conf.needFiles = strings.Split(buff, "|")
	buff = section.Key("needHashPath").String()
	conf.needHashPath = strings.Split(buff, "|")
	buff = section.Key("needHash").String()
	conf.needHash = strings.Split(buff, "|")
	conf.onlyFind, _ = section.Key("onlyFind").Bool()
	conf.advanceHashInNeedFiles, _ = section.Key("advanceHashInNeedFiles").Bool()
	return conf
}

type Kuls struct {
	ip                     string   //:= "127.0.0.1"                                                        //ip address server
	port                   string   //:= "80"                                                             // port server
	grMax                  int      //:= 128                                                             //максимальное количество потоков (до 1024)
	needFiles              []string //:= []string{".hide.txt", "$MFT", "ntuser.dat"}                 //подстроки которые надо искать в названиях файла
	needHashPath           []string //:= []string{"\\Users\\mars\\Desktop\\develop\\mftwithname"} //папки в которых надо подсчитать хэш
	needHash               []string //:= []string{"f726d50e8bf94b6747ea9b07236851eb"}                 //массив искомых хэшей
	onlyFind               bool     //:= true                                                         //индикатор обозначающий вывод всех хэшей в папках поиска
	advanceHashInNeedFiles bool     //:= false
}
