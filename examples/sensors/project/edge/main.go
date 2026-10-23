// Command edge reads a sensor and sends each reading to the cloud. With
// --replay it reads the readings from the files of a directory, one number
// per line, instead of a sensor.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reading is one value of one sensor.
type Reading struct {
	Sensor string  `json:"sensor"`
	Value  float64 `json:"value"`
}

// Replay returns the readings in the files of dir, in file name order.
func Replay(dir, sensor string) ([]Reading, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []Reading
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not a number", filepath.Base(f), line)
			}
			out = append(out, Reading{Sensor: sensor, Value: v})
		}
	}
	return out, nil
}

// Send posts one reading to the cloud's /readings.
func Send(client *http.Client, cloud string, r Reading) error {
	body, _ := json.Marshal(r)
	resp, err := client.Post(strings.TrimRight(cloud, "/")+"/readings", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("the cloud answered %s", resp.Status)
	}
	return nil
}

func main() {
	cloud := flag.String("cloud", "http://cloud:8000", "the cloud's address")
	replay := flag.String("replay", "", "a directory of readings to send instead of a sensor's")
	sensor := flag.String("sensor", "", "the sensor's name; the host name by default")
	interval := flag.Duration("interval", 200*time.Millisecond, "the time between two readings")
	flag.Parse()
	if *sensor == "" {
		*sensor, _ = os.Hostname()
	}
	if *replay == "" {
		log.Fatal("this build reads no hardware sensor; give --replay")
	}
	readings, err := Replay(*replay, *sensor)
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, r := range readings {
		for {
			if err := Send(client, *cloud, r); err != nil {
				log.Printf("send %v: %v; retrying", r.Value, err)
				time.Sleep(time.Second)
				continue
			}
			break
		}
		log.Printf("sent %s %v", r.Sensor, r.Value)
		time.Sleep(*interval)
	}
	log.Printf("sent %d readings; idle", len(readings))
	select {}
}
