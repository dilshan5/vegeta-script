package main

import (
	"encoding/json"
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2"
	vegeta "github.com/tsenart/vegeta/v12/lib"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

func main() {
	//get current time in GMT
	location, err := time.LoadLocation("GMT")
	if err != nil {
		fmt.Println(err)
	}

	rampUpRate1 := vegeta.Rate{Freq: 500, Per: time.Second}
	rampUpDuration1 := 300 * time.Second
	uniqueId := "Vegeta_Client_" + strconv.Itoa(int(rand.Intn(100000000))) + strconv.Itoa(int(time.Now().UnixNano()))
	// Create map of string slices.
	requestHeaders := map[string][]string{
		"Authorization":            {"Bearer token"},
		"vegeta-client-request-id": {uniqueId},
	}

	targeter := vegeta.NewStaticTargeter(vegeta.Target{
		Method: "GET",
		URL:    "http request",
		Header: requestHeaders,
	})

	/*	set the TimeOut as 1600 seconds
		set the maximum idle open connections per target host as 1000000
	*/
	attacker := vegeta.NewAttacker(vegeta.Timeout(1600*time.Second), vegeta.Connections(10000000))

	var rampUpMetrics vegeta.Metrics
	var wg sync.WaitGroup

	now := time.Now().In(location)
	line := "----------- Ramp-1 ----------- " + now.String() + " -----------"
	printSeparator(line, "rampingResponseErrors")

	for ramp1 := range attacker.Attack(targeter, rampUpRate1, rampUpDuration1, "Ramp-1 !!") {
		wg.Add(1)
		rampUpMetrics.Add(ramp1)
		go func(res *vegeta.Result) {
			defer wg.Done()
			if res.Code != 200 {
				printErrorResponse(res, "rampingResponseErrors")
			}
		}(ramp1)
	}

	rampUpRate2 := vegeta.Rate{Freq: 1000, Per: time.Second}
	rampUpDuration2 := 300 * time.Second

	now = time.Now().In(location)
	line = "----------- Ramp-2 ----------- " + now.String() + " -----------"
	printSeparator(line, "rampingResponseErrors")

	for ramp2 := range attacker.Attack(targeter, rampUpRate2, rampUpDuration2, "Ramp-2 !!") {
		wg.Add(1)
		rampUpMetrics.Add(ramp2)
		go func(res *vegeta.Result) {
			defer wg.Done()
			if res.Code != 200 {
				printErrorResponse(res, "rampingResponseErrors")
			}
		}(ramp2)
	}

	rampUpRate3 := vegeta.Rate{Freq: 1500, Per: time.Second}
	rampUpDuration3 := 300 * time.Second

	now = time.Now().In(location)
	line = "----------- Ramp-3 ----------- " + now.String() + " -----------"
	printSeparator(line, "rampingResponseErrors")

	for ramp3 := range attacker.Attack(targeter, rampUpRate3, rampUpDuration3, "Ramp-3 !!") {
		wg.Add(1)
		rampUpMetrics.Add(ramp3)
		go func(res *vegeta.Result) {
			defer wg.Done()
			if res.Code != 200 {
				printErrorResponse(res, "rampingResponseErrors")
			}
		}(ramp3)
	}

	rampUpMetrics.Close()
	printFinalMetrics(rampUpMetrics, "rampingResult")

	// ------------------------------------------------------------------------------------
	var loadMetrics vegeta.Metrics

	holdLoadRate := vegeta.Rate{Freq: 1600, Per: time.Second}
	holdLoadDuration := 1800 * time.Second

	now = time.Now().In(location)
	line = "----------- Load-1 ----------- " + now.String() + " -----------"
	printSeparator(line, "loadResponseErrors")

	for load1 := range attacker.Attack(targeter, holdLoadRate, holdLoadDuration, "Load-1 !!") {
		wg.Add(1)
		loadMetrics.Add(load1)
		go func(res *vegeta.Result) {
			defer wg.Done()
			if res.Code != 200 {
				printErrorResponse(res, "loadResponseErrors")
			}
			//	publishToInfluxDb(res)
		}(load1)
	}

	loadMetrics.Close()
	wg.Wait()
	printFinalMetrics(loadMetrics, "loadResults")

	fmt.Printf("rampMetrics: 99th percentile: %s\n", rampUpMetrics.Latencies.P99)
	fmt.Printf("loadMetrics: 99th percentile: %s\n", loadMetrics.Latencies.P99)
}

func printFinalMetrics(metric vegeta.Metrics, fileName string) {
	metrics, _ := json.MarshalIndent(metric, "", " ")
	sep := "\n"
	fileName = "./" + fileName + ".json"
	f, err := os.OpenFile(fileName, os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		panic(err)
	}

	_, err = f.WriteString(string(metrics))
	_, err = f.WriteString(sep)
}

// separate the each ramp up period logs
func printSeparator(separator string, fileName string) {
	sep := "\n"
	fileName = "./" + fileName + ".log"
	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0777)
	if err != nil {
		panic(err)
	}

	_, err = f.WriteString(separator)
	_, err = f.WriteString(sep)
}

func printErrorResponse(res *vegeta.Result, fileName string) {
	sep := "\n"
	fileName = "./" + fileName + ".log"
	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0777)
	if err != nil {
		panic(err)
	}

	// print Response Status
	_, err = f.WriteString("Status Code : " + strconv.Itoa(int(res.Code)))
	_, err = f.WriteString(sep)

	//print Response Headers
	for key, value := range res.Headers {
		_value := strings.Join(value, " ")
		_, err = f.WriteString(key + " : ")
		_, err = f.WriteString(_value)
		_, err = f.WriteString(sep)
	}

	// print Response Body
	body := BytesToString(res.Body)
	_, err = f.WriteString(body)
	_, err = f.WriteString(sep)
	_, err = f.WriteString(sep)

	if err != nil {
		panic(err)
	}

	defer f.Close()
}

/***
Convert Byte array to String
*/
func BytesToString(b []byte) string {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := reflect.StringHeader{Data: bh.Data, Len: bh.Len}
	return *(*string)(unsafe.Pointer(&sh))
}

func publishToInfluxDb(res *vegeta.Result) {
	applicationName := "bucket-name" // Bucket name. Do not use values starting with underscore (_)
	organizationName := "org-name"   //Org name
	token := "example-token"
	// Store the URL of your InfluxDB instance
	url := "http://xx.xx.xx.x:8086"

	// Create a new client using an InfluxDB server base URL and an authentication token
	client := influxdb2.NewClientWithOptions(url, token, influxdb2.DefaultOptions().
		SetUseGZip(true).SetBatchSize(100000))
	// Get non-blocking write client
	writeAPI := client.WriteAPI(organizationName, applicationName)
	// Get errors channel
	errorsCh := writeAPI.Errors()
	// Create go proc for reading and logging errors
	go func() {
		for err := range errorsCh {
			fmt.Printf("write error: %s\n", err.Error())
		}
	}()

	transactionName := "GET Request" //measurement name
	// write some points
	// create point
	p := influxdb2.NewPointWithMeasurement(transactionName).
		AddField("code", res.Code).
		AddField("latency", res.Latency.Nanoseconds()).
		AddField("bytes_out", strconv.Itoa(int(res.BytesOut))).
		AddField("bytes_in", strconv.Itoa(int(res.BytesIn))).
		SetTime(time.Now())
	// write asynchronously
	writeAPI.WritePoint(p)

	// Force all unwritten data to be sent
	writeAPI.Flush()
	// Ensures background processes finishes
	client.Close()
}
