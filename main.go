package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/influxdata/influxdb-client-go/v2"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func main() {
	rate := vegeta.Rate{Freq: 1, Per: time.Second}
	duration := 30 * time.Second
	// Create map of string slices.
	requestHeaders := map[string][]string{
		"Authorization": {"Bearer token"},
	}

	targeter := vegeta.NewStaticTargeter(vegeta.Target{
		Method: "GET",
		URL:    "http request",
		Header: requestHeaders,
	})

	//set the TimeOut as 300 seconds
	attacker := vegeta.NewAttacker(vegeta.Timeout(305 * time.Second))

	var metrics vegeta.Metrics
	var wg sync.WaitGroup

	for res := range attacker.Attack(targeter, rate, duration, "Big Bang!") {
		wg.Add(1)
		metrics.Add(res)
		go func(res *vegeta.Result) {
			defer wg.Done()
			if res.Code != 200 {
				printErrorResponse(res)
			}
			publishToInfluxDb(res)
		}(res)
	}
	metrics.Close()
	wg.Wait()
	printFinalMetrics(metrics)

	fmt.Printf("99th percentile: %s\n", metrics.Latencies.P99)
}

func printFinalMetrics(metric vegeta.Metrics) {
	metrics, _ := json.MarshalIndent(metric, "", " ")
	sep := "\n"
	f, err := os.OpenFile("./finalResult.json", os.O_APPEND|os.O_WRONLY, 0777)
	if err != nil {
		panic(err)
	}

	_, err = f.WriteString(string(metrics))
	_, err = f.WriteString(sep)
}

func printErrorResponse(res *vegeta.Result) {
	sep := "\n"
	f, err := os.OpenFile("./responseErrors.log", os.O_APPEND|os.O_WRONLY, 0777)
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
