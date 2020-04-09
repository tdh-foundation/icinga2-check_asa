// This file content implementation of methods to check CISCO 2960/Nexus switch
// interfaces status
package main

import (
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

//noinspection ALL
const (
	Up        = 1
	Down      = 0
	Exception = -1
)

var (
	Connected    = regexp.MustCompile(`(?i)^connected$`)
	NotConnected = regexp.MustCompile(`(?i)^notconnect?$`)
	Disabled     = regexp.MustCompile(`(?i)^disabled$`)
	ErrDisabled  = regexp.MustCompile(`(?i)^err-dis[a-zA-Z]+$`)
	XcrvrAbsen   = regexp.MustCompile(`(?i)^xcvrabsen$`)
	NoOperMem    = regexp.MustCompile(`(?i)^noOpermem$`)
	DownStatus   = regexp.MustCompile(`(?i)^down$`)

	// Status condtions
	OkCondition  = []*regexp.Regexp{Connected, NotConnected, Disabled, DownStatus, XcrvrAbsen}
	CriCondition = []*regexp.Regexp{ErrDisabled}
	WarCondition = []*regexp.Regexp{NoOperMem}

	// Metric conditions
	UpCondition   = []*regexp.Regexp{Connected}
	DownCondition = []*regexp.Regexp{NotConnected, Disabled, DownStatus, XcrvrAbsen}
)

type CiscoASA struct {
	Name string
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
	var reThreshold = regexp.MustCompile(`(\d*)[,|;](\d*)[,|;](\d*).*`)

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

	for _, cpu := range usageCPU {
		// Updating global status
		criticalThreshold := reThreshold.FindStringSubmatch(critical)
		warningThreshold := reThreshold.FindStringSubmatch(warning)
		cpu5s, _ := strconv.Atoi(cpu["cpu_5s"])
		cpu1m, _ := strconv.Atoi(cpu["cpu_1m"])
		cpu5m, _ := strconv.Atoi(cpu["cpu_5m"])

		if criticalThreshold != nil {
			cpu5sTH, _ := strconv.Atoi(criticalThreshold[1])
			cpu1mTH, _ := strconv.Atoi(criticalThreshold[2])
			cpu5mTH, _ := strconv.Atoi(criticalThreshold[3])

			if cpu5s > cpu5sTH {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5s CPU usage > %d%%", cpu5s)
			}
			if cpu1m > cpu1mTH {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("1m CPU usage > %d%%", cpu1m)
			}
			if cpu5m > cpu5mTH {
				condition = ict.CriExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5m CPU usage > %d%%", cpu5m)
			}
		}

		if warningThreshold != nil {
			cpu5sTH, _ := strconv.Atoi(warningThreshold[1])
			cpu1mTH, _ := strconv.Atoi(warningThreshold[2])
			cpu5mTH, _ := strconv.Atoi(warningThreshold[3])

			if cpu5s > cpu5sTH && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("5s CPU usage > %d%%", cpu5s)
			}
			if cpu1m > cpu1mTH && condition < ict.WarExit {
				condition = ict.WarExit
				if message != "" {
					message += "/"
				}
				message += fmt.Sprintf("1m CPU usage > %d%%", cpu1m)
			}
			if cpu5m > cpu5mTH && condition < ict.WarExit {
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
		// Updating global status
		//TODO:Memory updating status depending threshold

		// Setting CPU usage metrics
		metrics += fmt.Sprintf("'Free memory [%%]'=%s%% ", memory["percent_free_memory"])
		freeMem, _ := strconv.ParseFloat(memory["free_memory"], 64)
		metrics += fmt.Sprintf("'Free memory [MB]'=%.2fMB ", freeMem/math.Pow(1024, 2))
	}

	// Print log values if program is called in Test mode
	if os.Getenv("CHECK_MODE") == "TEST" {
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
