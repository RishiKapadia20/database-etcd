// AUTHORS:
//     Copyright (C) 2003-2018 Opsview Limited. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// This file is part of Opsview
//
// This plugin monitors etcd

package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/opsview/go-plugin"
)

var opts struct {
	// Settings for flags for when running the program
	Hostname   string `short:"H" long:"hostname" description:"Host" default:"localhost"`
	Port       string `short:"P" long:"port" description:"Port" default:"2379"`
	Mode       string `short:"m" long:"mode" description:"Mode" required:"true"`
	Warning    string `short:"w" long:"warning" description:"Warning"`
	Critical   string `short:"c" long:"critical" description:"Critical"`
	Debug      bool   `short:"d" long:"debug" description:"Enables debugger mode"`
	APIVersion string `short:"a" long:"API-Version" description:"ETCD API Version"`
}

type versionStruc struct {
	// Used for storing version details when retrieved
	Version string `json:"etcdserver"`
}

func main() {
	// Main function creates the new plugin instance, then calls function to retrieve metrics
	// For the required metric it retrieves the stat and adds it to the output
	// Switches into various cases depending on which metric is needed

	// Creates new instance of a plugin and sets up output for plugin
	check := checkPlugin()

	if err := check.ParseArgs(&opts); err != nil {
		// Handles error if incorrect arguments entered
		check.ExitUnknown("Error parsing arguments: %s", err)
	}

	defer check.Final() // Calculates the final output after function completes

	check.AllMetricsInOutput = true // So that all metrics are added to the check message

	metrics := retrieveMetrics(check, opts.Hostname, opts.Port) // Saves output from host:port/metrics

	version, minorVersion := findVersion(check) // Saves the version of etcd the host is running

	switch opts.Mode {
	// Depending on which metric is requested, output its value with its UOM and thresholds

	// Below modes used by etcd v3 only
	case "client_sent_and_received_bytes":
		if version == "2" {
			check.ExitUnknown("Unsupported version of etcd, this metric is only available in version 3.0.0 or higher")
		}

		sentBytes := findMetric(check, "client_grpc_sent_bytes_total", metrics)
		check.AddMetric("client_sent_bytes", sentBytes, "c")

		receivedBytes := findMetric(check, "client_grpc_received_bytes_total", metrics)
		check.AddMetric("client_received_bytes", receivedBytes, "c")

	case "leader_changes_seen":
		if version == "2" {
			check.ExitUnknown("Unsupported version of etcd, this metric is only available in version 3.0.0 or higher")
		}

		leaderChangesSeen := findMetric(check, "leader_changes_seen_total", metrics)
		check.AddMetric("leader_changes_seen", leaderChangesSeen, "c", opts.Warning, opts.Critical)

	// Below modes used by etcd v2 and v3
	case "durations":
		var fsyncDuration string

		if version == "2" {
			fsyncDuration = findAverage(check, "fsync_durations_seconds_sum", "fsync_durations_seconds_count", metrics)
		} else {
			backendCommitDuration := findAverage(check, "backend_commit_duration_seconds_sum", "backend_commit_duration_seconds_count", metrics)
			check.AddMetric("backend_commit_avg_duration", backendCommitDuration, "s")

			fsyncDuration = findAverage(check, "fsync_duration_seconds_sum", "fsync_duration_seconds_count", metrics)
		}
		check.AddMetric("fsync_avg_duration", fsyncDuration, "s")

	case "file_descriptors":
		openFds := findMetric(check, "process_open_fds", metrics)
		check.AddMetric("open_fds", openFds, "fd")

		maxFds := findMetric(check, "process_max_fds", metrics)
		check.AddMetric("max_fds", maxFds, "fd")

	case "http_requests":
		if version == "3" && minorVersion == "3" {
			check.ExitUnknown("Unsupported version of etcd, this metric is not available in version 3.3")
		}
		httpRequests := findMetric(check, "http_requests_total{code=\"200\",handler=\"prometheus\",method=\"get\"}", metrics)
		check.AddMetric("http_requests", httpRequests, "c", opts.Warning, opts.Critical)

	case "proposals":
		if version == "3" {

			proposalsCommitted := findMetric(check, "proposals_committed_total", metrics)
			check.AddMetric("proposals_committed", proposalsCommitted, "c")

			proposalsApplied := findMetric(check, "proposals_applied_total", metrics)
			check.AddMetric("proposals_applied", proposalsApplied, "c")
		}

		var proposalsPending string

		if version == "2" {
			proposalsPending = findMetric(check, "pending_proposal_total", metrics)
		} else {
			proposalsPending = findMetric(check, "proposals_pending", metrics)
		}
		check.AddMetric("proposals_pending", proposalsPending, "")

		var proposalsFailed string

		if version == "2" {
			proposalsFailed = findMetric(check, "proposal_failed_total", metrics)
		} else {
			proposalsFailed = findMetric(check, "proposals_failed_total", metrics)
		}
		check.AddMetric("proposals_failed", proposalsFailed, "")

	case "watchers":
		var watchers string
		if version == "2" || opts.APIVersion == "2" {
			watchers = findMetric(check, "store_watchers", metrics)
		} else {
			watchers = findMetric(check, "mvcc_watcher_total", metrics)
		}
		check.AddMetric("watchers", watchers, "", opts.Warning, opts.Critical)

	// Below are used to retrieve individual metrics that are grouped together in the above cases

	// Below modes used by etcd v3 only
	case "proposals_committed":
		if version == "2" {
			check.ExitUnknown("Unsupported version of etcd, this metric is only available in version 3.0.0 or higher")
		}

		proposalsCommitted := findMetric(check, "proposals_committed_total", metrics)
		check.AddMetric("proposals_committed", proposalsCommitted, "c", opts.Warning, opts.Critical)

	case "proposals_applied":
		if version == "2" {
			check.ExitUnknown("Unsupported version of etcd, this metric is only available in version 3.0.0 or higher")
		}

		proposalsApplied := findMetric(check, "proposals_applied_total", metrics)
		check.AddMetric("proposals_applied", proposalsApplied, "c", opts.Warning, opts.Critical)

	case "backend_commit_avg_duration":
		if version == "2" {
			check.ExitUnknown("Unsupported version of etcd, this metric is only available in version 3.0.0 or higher")
		}

		backendCommitDuration := findAverage(check, "backend_commit_duration_seconds_sum", "backend_commit_duration_seconds_count", metrics)
		check.AddMetric("backend_commit_avg_duration", backendCommitDuration, "s", opts.Warning, opts.Critical)

	// Below modes used by etcd v2 and v3
	case "proposals_pending":
		var proposalsPending string

		if version == "2" {
			proposalsPending = findMetric(check, "pending_proposal_total", metrics)
		} else {
			proposalsPending = findMetric(check, "proposals_pending", metrics)
		}
		check.AddMetric("proposals_pending", proposalsPending, "", opts.Warning, opts.Critical)

	case "proposals_failed":
		var proposalsFailed string

		if version == "2" {
			proposalsFailed = findMetric(check, "proposal_failed_total", metrics)
		} else {
			proposalsFailed = findMetric(check, "proposals_failed_total", metrics)
		}
		check.AddMetric("proposals_failed", proposalsFailed, "", opts.Warning, opts.Critical)

	case "process_open_fds":
		openFds := findMetric(check, "process_open_fds", metrics)
		check.AddMetric("open_fds", openFds, "fd", opts.Warning, opts.Critical)

	case "process_max_fds":
		maxFds := findMetric(check, "process_max_fds", metrics)
		check.AddMetric("max_fds", maxFds, "fd", opts.Warning, opts.Critical)

	case "fsync_avg_durations":
		var fsyncDuration string

		if version == "2" {
			fsyncDuration = findAverage(check, "fsync_durations_seconds_sum", "fsync_durations_seconds_count", metrics)
		} else {
			fsyncDuration = findAverage(check, "fsync_duration_seconds_sum", "fsync_duration_seconds_count", metrics)
		}
		check.AddMetric("fsync_avg_duration", fsyncDuration, "s", opts.Warning, opts.Critical)

	default:
		check.ExitUnknown("Incorrect check mode. Please check help text for options (-h)")
	}
}

func retrieveMetrics(check *plugin.Plugin, host string, port string) string {
	// Function used to retrieve metrics from the etcd host
	// Takes the host and port as input
	// Returns a string of the metrics for etcd

	var resp *http.Response
	var err error

	url := host + ":" + port + "/metrics"

	// Connect to the etcd host and retrieve metrics
	resp, err = http.Get("http://" + url)
	if err != nil {
		check.ExitUnknown("Error: Could not connect to etcd server: http://"+url+" %s", err)
	}

	defer resp.Body.Close() // Closes the response body when finished with it

	body, err := ioutil.ReadAll(resp.Body) // Reads the data retrieved in the response
	if err != nil {
		// If unable to read the data retrieve, exit unknown
		check.ExitUnknown("Could not read response from "+host+":"+port+" %s", err)
	}

	metrics := string(body[:]) // Convert bytes array of characters to a string

	return metrics
}

func findVersion(check *plugin.Plugin) (string, string) {
	// Sends get request for version of etcd that is installed
	// Returns string of the major version number

	var resp *http.Response
	var err error

	url := opts.Hostname + ":" + opts.Port + "/version"

	// Connect to the etcd host and retrieve version details, use https if requested
	resp, err = http.Get("http://" + url)
	if err != nil {
		check.ExitUnknown("Error: Could not connect to etcd server: http://"+url+" %s", err)
	}

	defer resp.Body.Close()

	// Raw data from http request
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		check.ExitUnknown("Error: Problem reading from API %s", err)
	}

	// Uses custom type to get version number from response
	var versionOutput versionStruc
	err = json.Unmarshal(body, &versionOutput)
	if err != nil {
		check.ExitUnknown("Problem with unmarshaling json data: %s", err)
	}

	versions := strings.Split(string(versionOutput.Version), ".")

	return versions[0], versions[1]
}

func findMetric(check *plugin.Plugin, metricName string, message string) string {
	// Function used for finding a metric value in the message output - that is a float value
	// Only returns the value for the given metric name

	regularExpression, err := regexp.Compile(metricName + "\\s[0-9]+.*") // Regular expression for search criteria and the number following it
	if err != nil {
		// If unable to compile expressions, exit unknown
		check.ExitUnknown("Can't compile regular expression")
	}

	foundStat := regularExpression.FindString(message) // Search expression on message

	foundMetric := strings.TrimPrefix(foundStat, metricName+" ")

	foundMetricFloat, err := strconv.ParseFloat(foundMetric, 64) // Convert to float for this specific case
	if err != nil {
		// Handles error if cannot convert to a float
		check.ExitUnknown("Failed to convert value for " + metricName + " to float")
	}

	foundMetricString := strconv.FormatFloat(foundMetricFloat, 'f', -1, 64)

	return foundMetricString
}

func findAverage(check *plugin.Plugin, sumName string, countName string, metrics string) string {
	// Function used to find the average of metrics results
	// Takes in the metric sum name, and the count name
	// Find the sum and count in the metrics output, calculates the average, then adds this to the check output

	durationSum := findMetric(check, sumName, metrics)

	diffDurationSum := getDifferenceInValues(check, sumName, durationSum)

	durationCount := findMetric(check, countName, metrics)

	diffDurationCount := getDifferenceInValues(check, countName, durationCount)

	if opts.Debug == true {
		fmt.Print(sumName + " - durationSum: ")
		fmt.Println(durationSum)

		fmt.Print(sumName + " - diffdurationSum: ")
		fmt.Println(diffDurationSum)

		fmt.Print(countName + " - durationCount: ")
		fmt.Println(durationCount)

		fmt.Print(countName + " - diffdurationCount: ")
		fmt.Println(diffDurationCount)
	}

	var durationAverage float64

	durationAverage = diffDurationSum / diffDurationCount

	if diffDurationCount == 0.0 || diffDurationSum == 0.0 {
		durationAverage = 0.0
	}

	durationAverageString := strconv.FormatFloat(durationAverage, 'f', 4, 64)

	return durationAverageString
}

func getDifferenceInValues(check *plugin.Plugin, metricName string, metricValue string) float64 {
	// Function takes in a metric name and its value
	// Saves the value to a file and calculates the difference between the current value and the previous one
	// Returns the difference in the current and previous value as an int

	previousValue := updateState(check, opts.Hostname, opts.Port, metricName, metricValue)

	// Catch expected error for first run as there is no previous value
	if previousValue == "" {
		check.ExitUnknown("Unable to process previous result. Data will be collected on next execution. This is expected as this is first run.")
	}

	previousValueFloat, err := strconv.ParseFloat(previousValue, 64)
	if err != nil {
		filePath, err := checkFilePath(opts.Hostname, opts.Port, opts.Mode) // Find path that was used for this value
		if err != nil {
			check.ExitUnknown("Error finding path that was used: %s", err)
		}

		// Delete file at this path, otherwise this error will always trigger and the value on the file will never change
		os.Remove(filePath)

		check.ExitUnknown("Error processing cached previous result.")
	}

	metricValueFloat, err := strconv.ParseFloat(metricValue, 64)

	if opts.Debug == true {
		fmt.Print("Metric Value: ")
		fmt.Println(metricValue)

		fmt.Print("Previous Value: ")
		fmt.Println(previousValueFloat)
	}

	changeInValue := metricValueFloat - previousValueFloat

	return changeInValue
}

func updateState(check *plugin.Plugin, HostAddress string, Port string, Mode string, records string) string {
	// File is used to store temporary value for each metric to monitor changes between values
	// Everything that involves the file is run here
	// Opens the file a so it can be read, then writes a new file with the current records
	// Returns the value that was stored on the file

	path, err := checkFilePath(HostAddress, Port, Mode) // Provides a path for the file to be saved to
	if err != nil {
		check.ExitUnknown("Error finding temporary path: %s", err)
	}

	// Opens file in read and write mode, creates it if not already there and gives 600 permission levels
	file, err := os.OpenFile(path, syscall.O_RDWR|syscall.O_CREAT, 0600)
	if err != nil {
		// If there is an error opening/creating the file, exit unknown
		check.ExitUnknown("Error creating temporary file: " + path)
	}

	defer file.Close() // Closes the file after using it

	getLock(check, file, path) // Uses getLock() to prevent access to file while using it

	defer releaseLock(file) // Uses releaseLock() after using file to allow access again

	fileBytes, err := ioutil.ReadFile(path) // Reads the file contents at the given path
	if err != nil {
		// If there is an error reading the file, exit unknown
		check.ExitUnknown("Cannot read previous metrics from temporary file: "+path+" %s", err)
	}

	previousValues := string(fileBytes) // Saves the value after opening a previous value

	file.Truncate(0)

	w := bufio.NewWriter(file)
	_, err = w.WriteString(records)
	if err != nil {
		// If there is an error, exit unknown
		check.ExitUnknown("Error writing to temporary file: "+path+" %s", err)
	}

	w.Flush()

	return previousValues
}

func getLock(check *plugin.Plugin, file *os.File, path string) {
	// Set the lock on the file so no other processes can read or write to it

	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)

	if err != nil {
		// If there is an error, exit unknown
		check.ExitUnknown("Error locking temporary file: "+path+" %s", err)
	}
}

func releaseLock(file *os.File) {
	// Release file lock

	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func checkFilePath(HostAddress string, Port string, Mode string) (string, error) {
	// Tests the locations of the path to check the file can be written
	// Creates a file name using md5 to create a hash
	// Returns a string of the path for the file to be written to

	fileName := "etcd_"
	hash := HostAddress + "," + Port + "," + Mode

	fileName = fileName + GetMD5Hash(hash) + ".tmp"

	env := os.Getenv("OPSVIEW_BASE")

	paths := []string{"/opt/opsview/agent/tmp/",
		"/opt/opsview/monitoringscripts/tmp/",
		env + "/tmp/",
		"/tmp/"}

	var failedPaths string

	for _, path := range paths {
		// For all paths we can use for temp files

		if _, err := os.Stat(path); err == nil {
			// If temp path exists and user has permissions to read and write, return this path and filename
			return path + fileName, err
		} else {
			failedPaths += path + " or "
		}
	}

	// Return error if none of the paths available are valid
	err := errors.New("Unable to create temporary file in path(s): " + failedPaths[:len(failedPaths)-4])

	return "", err
}

func GetMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func checkPlugin() *plugin.Plugin {
	// Creates new instance of a plugin
	// Sets the plugin name, version, preamble, and help text description
	// Function returns a new check plugin object

	check := plugin.New("check_etcd", "v1.0.0")

	check.Preamble = `Copyright (c) 2003-2018 Opsview Limited. All rights reserved.
					 This plugin tests the stats of an etcd server.`

	check.Description = `etcd Monitoring Opspack Plugin supports the following metrics:

	Modes used for etcd v3 only:

	client_sent_and_received_bytes:		Sent and Received bytes from client
	durations:				The latency distributions of commit called by backend and latency distributions of fsync called by wal
	leader_changes_seen:		The number of changes to the leader

	Modes used by etcd v2 and v3:

	file_descriptors:		Number of maximum and open file descriptors
	http_requests:			Total number of HTTP requests made
	proposals:			Number of proposals applied, committed and pending to the system
	watchers:			Number of current watchers

	Optional modes to view metrics individually - v3 only:

	backend_commit_avg_duration:		The latency distributions of commit called by backend
	proposals_applied:			Total proposals applied to the system
	proposals_committed:			Total proposals committed to the system

	Optional modes to view metrics individually - v2 and v3:

	fsync_avg_durations:			The latency distributions of fsync called by wal
	process_max_fds:			Max number of file descriptors
	process_open_fds:			Number of open file descriptors
	proposals_failed:			Failed proposal count
	proposals_pending:			Pending proposals waiting for commit`

	return check
}
