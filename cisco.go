// This file content implementation of methods to check CISCO 2960/Nexus switch
// interfaces status
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	//"fmt"
	ict "github.com/tdh-foundation/icinga2-go-checktools"
	"log"
	"os"
	"regexp"
	"strconv"
	//"strings"
)

type CiscoASA struct {
	Name string
}

type Threshold struct {
	CPU      []int `json:"cpu,omitempty"`
	Memory   int   `json:"memory,omitempty"`
	UsersVPN int   `json:"users_vpn,omitempty"`
}

// Instantiate a new CiscoASA
func NewCiscoASA(name string) *CiscoASA {
	ca := new(CiscoASA)
	ca.Name = name
	return ca
}

// CheckStatus check Cisco ASA environment conditions
func (asa *CiscoASA) CheckStatus(host string, username string, password string, identity string, port int, critical string, warning string) (ict.Icinga, error) {

	var ssh *ict.SSHTools

	var reCooling = regexp.MustCompile(`(?mi)^\s*cooling Fan\s+(?P<number>\d+)\s*:\s+(?P<rpm>\d+)\s+RPM\s+-\s+(?P<status>.+)$`)
	var reCPUTemp = regexp.MustCompile(`(?mi)^\s*Processor\s+(?P<number>\d+):\s*(?P<temp>\d+\.\d)\s+C\s+-\s+(?P<status>.+)$`)
	var reAmbient = regexp.MustCompile(`(?mi)^\s*Ambient\s+(?P<number>\d+):\s*(?P<temp>\d+\.\d)\s+C\s+-\s+(?P<status>.*)\s+\((?P<name>.*)\)\s*$`)
	var reCPU = regexp.MustCompile(`(?mi)^CPU utilization for.*=\s*(?P<cpu_5s>\d*)%;.*:\s*(?P<cpu_1m>\d*)%;.*:\s*(?P<cpu_5m>\d*)%\s*$`)
	var reMem = regexp.MustCompile(`(?mi)^Free memory:\s+(?P<free_memory>\d+).+\((?P<percent_free_memory>\d*)%\)\s*$`)

	var warningTH Threshold
	var criticalTH Threshold

	// Opening a ssh session to the cisco ASA
	ssh, err = ict.NewSSHTools(host, username, password, identity, port)
	if err != nil {
		return ict.Icinga{}, err
	}

	// Sending commands to the Cisco ASA and getting returned data
	err = ssh.SendSSHhasPTY([]string{"enable\n\n", "terminal pager 0\n", "show environment\n", "show cpu\n", "show mem\n"}, `(?i)^(.*\>.?)|(.*\#.?)|(Password:.?)$`)
	if err != nil {
		return ict.Icinga{}, err
	}

	//
	// Parsing returned data
	//
	// Creating map for cooling conditions
	keys := reCooling.SubexpNames()[1:]
	var rpmCooling []map[string]string
	for _, s := range reCooling.FindAllStringSubmatch(ssh.Stdout, -1) {
		cooling := make(map[string]string)
		for i, v := range s[1:] {
			cooling[keys[i]] = v
		}
		rpmCooling = append(rpmCooling, cooling)
	}

	// Creating map for CPU temperature
	keys = reCPUTemp.SubexpNames()[1:]
	var tempCPU []map[string]string
	for _, s := range reCPUTemp.FindAllStringSubmatch(ssh.Stdout, -1) {
		cpu := make(map[string]string)
		for i, v := range s[1:] {
			cpu[keys[i]] = v
		}
		tempCPU = append(tempCPU, cpu)
	}

	// Creating map for ambient temperature
	keys = reAmbient.SubexpNames()[1:]
	var tempAmbient []map[string]string
	for _, s := range reAmbient.FindAllStringSubmatch(ssh.Stdout, -1) {
		ambient := make(map[string]string)
		for i, v := range s[1:] {
			ambient[keys[i]] = v
		}
		tempAmbient = append(tempAmbient, ambient)
	}

	// Creating map for CPU usage
	keys = reCPU.SubexpNames()[1:]
	var usageCPU []map[string]string
	for _, s := range reCPU.FindAllStringSubmatch(ssh.Stdout, -1) {
		cpu := make(map[string]string)
		for i, v := range s[1:] {
			cpu[keys[i]] = v
		}
		usageCPU = append(usageCPU, cpu)
	}

	// Creating map for free memory information
	keys = reMem.SubexpNames()[1:]
	var usageMemory []map[string]string
	for _, s := range reMem.FindAllStringSubmatch(ssh.Stdout, -1) {
		memory := make(map[string]string)
		for i, v := range s[1:] {
			memory[keys[i]] = v
		}
		usageMemory = append(usageMemory, memory)
	}

	// Set exit condition depending status of all probes
	var condition int = ict.OkExit
	var message string = ""
	var metrics string = ""

	for _, ambient := range tempAmbient {
		// Updating global status
		if strings.TrimSpace(ambient["status"]) != "OK" {
			condition = ict.CriExit
			if message != "" {
				message += "/"
			}
			message += fmt.Sprintf("Ambient %s temperature issue %s (%s °C)", ambient["name"], ambient["status"], ambient["temp"])
		}
		// Setting metrics
		metrics += fmt.Sprintf("'%s [°C]'=%s ", ambient["name"], ambient["temp"])
	}

	for _, cpu := range tempCPU {
		// Updating global status
		if strings.TrimSpace(cpu["status"]) != "OK" {
			condition = ict.CriExit
			if message != "" {
				message += "/"
			}
			message += fmt.Sprintf("CPU %s temperature issue %s (%s °C)", cpu["number"], cpu["status"], cpu["temp"])
		}
		// Setting metrics
		metrics += fmt.Sprintf("'CPU %s [°C]'=%s ", cpu["number"], cpu["temp"])
	}

	// Converting critical and warning threshold  JSON strings to Structured data
	errCritical := json.Unmarshal([]byte(critical), &criticalTH)
	errWarning := json.Unmarshal([]byte(warning), &warningTH)

	for _, cpu := range usageCPU {
		// Updating global status
		cpu5s, _ := strconv.Atoi(cpu["cpu_5s"])
		cpu1m, _ := strconv.Atoi(cpu["cpu_1m"])
		cpu5m, _ := strconv.Atoi(cpu["cpu_5m"])

		if errCritical == nil && criticalTH.CPU != nil && len(criticalTH.CPU) == 3 {
			if cpu5s > criticalTH.CPU[0] {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5s CPU usage > %d%%", cpu5s)
			}
			if cpu1m > criticalTH.CPU[1] {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("1m CPU usage > %d%%", cpu1m)
			}
			if cpu5m > criticalTH.CPU[2] {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5m CPU usage > %d%%", cpu5m)
			}
		}

		if errWarning == nil && warningTH.CPU != nil && len(warningTH.CPU) == 3 {
			if cpu5s > warningTH.CPU[0] && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5s CPU usage > %d%%", cpu5s)
			}
			if cpu1m > warningTH.CPU[1] && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("1m CPU usage > %d%%", cpu1m)
			}
			if cpu5m > warningTH.CPU[2] && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5m CPU usage > %d%%", cpu5m)
			}
		}

		// Setting CPU usage metrics
		metrics += fmt.Sprintf("'CPU usage [5s]'=%s%% ", cpu["cpu_5s"])
		metrics += fmt.Sprintf("'CPU usage [1m]'=%s%% ", cpu["cpu_1m"])
		metrics += fmt.Sprintf("'CPU usage [5m]'=%s%% ", cpu["cpu_5m"])
	}

	for _, cooling := range rpmCooling {
		if strings.TrimSpace(cooling["status"]) != "OK" {
			condition = ict.CriExit
			if message != "" {
				message += "/"
			}
			message += fmt.Sprintf("Cooling Fan %s issue %s (%s RPM)", cooling["number"], cooling["status"], cooling["rpm"])
		}
		// Setting metrics
		metrics += fmt.Sprintf("'Fan %s [RPM]'=%s ", cooling["number"], cooling["rpm"])
	}

	for _, memory := range usageMemory {
		freeMem, _ := strconv.ParseFloat(memory["free_memory"], 64)
		percFreeMem, _ := strconv.Atoi(memory["percent_free_memory"])

		// Updating global status
		if errCritical == nil && criticalTH.Memory > 0 {
			if percFreeMem < criticalTH.Memory {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("Free memory %d%% lower than %d%%", percFreeMem, criticalTH.Memory)
			}
		}

		if errWarning == nil && criticalTH.Memory > 0 {
			if percFreeMem < warningTH.Memory && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("Free memory %d%% lower than %d%%", percFreeMem, criticalTH.Memory)
			}
		}

		// Setting CPU usage metrics
		metrics += fmt.Sprintf("'Free memory [%%]'=%d%% ", percFreeMem)
		metrics += fmt.Sprintf("'Free memory [MB]'=%.2fMB ", freeMem/math.Pow(1024, 2))
	}

	// Print log values if program is called in Test mode
	if os.Getenv("VERBOSE") == "TRUE" {
		for _, ambient := range tempAmbient {
			log.Printf("%s - %s°C - %s.", ambient["name"], ambient["temp"], strings.TrimSpace(ambient["status"]))
		}

		for _, cpu := range tempCPU {
			log.Printf("CPU %s - %s°C - %s\n", cpu["number"], cpu["temp"], cpu["status"])
		}
		for _, cooling := range rpmCooling {
			log.Printf("Fan %s - %s RPM - %s\n", cooling["number"], cooling["rpm"], cooling["status"])
		}

		for _, cpu := range usageCPU {
			log.Printf("CPU usage 5s %s%%, 1m %s%%, 5m %s%%\n", cpu["cpu_5s"], cpu["cpu_1m"], cpu["cpu_5m"])
		}

		for _, memory := range usageMemory {
			freeMem, _ := strconv.ParseFloat(memory["free_memory"], 64)
			log.Printf("Free memory %.2fMB %s%%\n", freeMem/math.Pow(1024, 2), memory["percent_free_memory"])
		}
	}

	if message == "" {
		message = "Everything is Ok"
	}
	return ict.Icinga{Message: message, Exit: condition, Metric: metrics}, err
}

func (asa *CiscoASA) CheckVPNUsers(host string, username string, password string, identity string, port int, critical string, warning string) (ict.Icinga, error) {

	var ssh *ict.SSHTools

	var reUsers = regexp.MustCompile(`(?mi)^remote access VPN user.*\'(?P<username>.*)\'.*$`)

	var warningTH Threshold
	var criticalTH Threshold

	// Opening a ssh session to the cisco ASA
	ssh, err = ict.NewSSHTools(host, username, password, identity, port)
	if err != nil {
		return ict.Icinga{}, err
	}

	// Sending commands to the Cisco ASA and getting returned data
	err = ssh.SendSSHhasPTY([]string{"enable\n\n", "terminal pager 0\n", "show uauth | include remote access VPN user\n"}, `(?i)^(.*\>.?)|(.*\#.?)|(Password:.?)$`)
	if err != nil {
		return ict.Icinga{}, err
	}

	//
	// Parsing returned data
	//
	// Creating map for users information information
	keys := reUsers.SubexpNames()[1:]
	var users []map[string]string
	for _, s := range reUsers.FindAllStringSubmatch(ssh.Stdout, -1) {
		user := make(map[string]string)
		for i, v := range s[1:] {
			user[keys[i]] = v
		}
		users = append(users, user)
	}

	// Set exit condition depending status of all probes
	var condition int = ict.OkExit
	var message string = ""
	var metrics string = ""

	// Converting critical and warning threshold  JSON strings to Structured data
	errCritical := json.Unmarshal([]byte(critical), &criticalTH)
	errWarning := json.Unmarshal([]byte(warning), &warningTH)

	if errWarning == nil && warningTH.UsersVPN > 0 {
		if len(users) > warningTH.UsersVPN {
			condition = ict.WarExit
			message = fmt.Sprintf("%d VPN remote connected users > %d", len(users), warningTH.UsersVPN)
		}
	}

	if errCritical == nil && criticalTH.UsersVPN > 0 {
		if len(users) > criticalTH.UsersVPN {
			condition = ict.CriExit
			message = fmt.Sprintf("%d VPN remote connected users > %d", len(users), criticalTH.UsersVPN)
		}
	}

	// Setting CPU usage metrics
	metrics += fmt.Sprintf("'Active users'=%d ", len(users))

	// Print log values if program is called in Test mode
	if os.Getenv("VERBOSE") == "TRUE" {
		for idx, user := range users {
			log.Printf("%d - %s", idx, user["username"])
		}
	}

	if message == "" {
		message = fmt.Sprintf("%d VPN remote connected users", len(users))
	}
	return ict.Icinga{Message: message, Exit: condition, Metric: metrics}, err
}

func (asa *CiscoASA) CheckFailover(host string, username string, password string, identity string, port int, critical string, warning string) (ict.Icinga, error) {

	var ssh *ict.SSHTools
	var reFailoverOn = regexp.MustCompile(`(?mi)^Failover (?P<status>.*)\s*$`)
	var reFailoverLink = regexp.MustCompile(`(?mi)^Failover LAN Interface:.*\((?P<failover_state>.*)\)\s*$`)
	/*
		var reLastFailover = regexp.MustCompile(`(?mi)^Last Failover at:\s(?P<time>\d{2}:\d{2}:\d{2}\s[A-Z]{1,3}[T]\s\w{3}\s\d{1,2}\s\d{4})\s*$`)
		var reThisHost = regexp.MustCompile(`(?mi)^\s*This host:\s(?P<host>\w*)\s*\-\s*(?P<state>.*)\s\s*$`)
		var reOtherHost = regexp.MustCompile(`(?mi)^\s*This host:\s(?P<host>\w*)\s*\-\s*(?P<state>.*)\s\s*$`)
		var reActiveTime = regexp.MustCompile(`(?mi)^\s*Active time:\s(?P<duration>\d*)\s*\(sec\)\s*$`)
	*/
	//var warningTH Threshold
	//var criticalTH Threshold

	// Set exit condition depending status of all probes
	var condition int = ict.OkExit
	var message string = ""
	var metrics string = ""

	// Opening a ssh session to the cisco ASA
	ssh, err = ict.NewSSHTools(host, username, password, identity, port)
	if err != nil {
		return ict.Icinga{}, err
	}

	// Sending commands to the Cisco ASA and getting returned data
	err = ssh.SendSSHhasPTY([]string{"enable\n\n", "terminal pager 0\n", "show failover\n"}, `(?i)^(.*\>.?)|(.*\#.?)|(Password:.?)$`)
	if err != nil {
		return ict.Icinga{}, err
	}

	//
	// Parsing returned data
	//

	//Checking if Failover is On (First line of response)
	failoverOn := reFailoverOn.FindStringSubmatch(ssh.Stdout)
	if failoverOn != nil {
		if strings.ToUpper(strings.TrimSpace(failoverOn[1])) != "ON" {
			condition = ict.CriExit
			message = "Failover status not On"
		}
	} else {
		condition = ict.CriExit
		message = "Failover status not found"
	}
	// If failover is not On or no information are returned exiting with Critical status
	if condition == ict.CriExit {
		return ict.Icinga{Message: message, Exit: condition, Metric: metrics}, err
	}

	//Checking if Failover link is Up
	failoverLink := reFailoverLink.FindStringSubmatch(ssh.Stdout)
	if failoverLink != nil {
		if strings.ToUpper(strings.TrimSpace(failoverLink[1])) != "UP" {
			condition = ict.CriExit
			message += fmt.Sprintf("Failover LAN Interface status %s", strings.TrimSpace(failoverLink[1]))
		}
	} else {
		condition = ict.CriExit
		message = "Failover link information not found"
		return ict.Icinga{Message: message, Exit: condition, Metric: metrics}, err
	}

	/*
		// Converting critical and warning threshold  JSON strings to Structured data
		errCritical := json.Unmarshal([]byte(critical), &criticalTH)
		errWarning := json.Unmarshal([]byte(warning), &warningTH)
	*/
	if message == "" {
		message = "Failover is up and running"
	}
	return ict.Icinga{Message: message, Exit: condition, Metric: metrics}, err
}
