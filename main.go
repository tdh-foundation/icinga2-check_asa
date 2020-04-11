// check_asa is a icinga/nagios check plugin who
// get information about status of CISCO ASA applicance (currently tested with CISCO ASA 5515)
package main

import (
	"fmt"
	"github.com/docopt/docopt-go"
	ict "github.com/tdh-foundation/icinga2-go-checktools"
	"os"
	"strconv"
)

// version of program
const version = "1.0.0"

var (
	arguments  docopt.Opts
	err        error
	buildcount string
	usage      string

	params struct {
		command    string
		host       string
		port       int
		username   string
		password   string
		identity   string
		version    bool
		verbose    bool
		switchType string
		critical   string
		warning    string
	}
)

// Init parsing program arguments
func init() {

	usage = `check_ciscoasa
Check CISCO ASA status
Usage: 
	check_ciscoasa (-h | --help | --version)
	check_ciscoasa status (-H <host> | --host=<host>) (-u <username> | --username=<username>) (-c <critical> | --critical=<critical>) (-w <warning> | --warning=<warning>) [-p <password> | --password=<password> | -i <pkey_file> | --identity=<pkey_file] [-P <port> | --port=<port>] [--verbose] 
	check_ciscoasa vpnusers (-H <host> | --host=<host>) (-u <username> | --username=<username>) (-c <critical> | --critical=<critical>) (-w <warning> | --warning=<warning>) [-p <password> | --password=<password> | -i <pkey_file> | --identity=<pkey_file] [-P <port> | --port=<port>] [--verbose] 
	check_ciscoasa failover (-H <host> | --host=<host>) (-u <username> | --username=<username>) [(-c <critical> | --critical=<critical>) (-w <warning> | --warning=<warning>)] [-p <password> | --password=<password> | -i <pkey_file> | --identity=<pkey_file] [-P <port> | --port=<port>] [--verbose] 
Options:
	--version  				Show check_ciscoasa version.
	-h --help  				Show this screen.
	-v --verbose  	Verbose mode
	-H <host> --host=<host>  		ASA hostname or IP Address
	-u <username> --username=<username>  	Username
	-p <password> --password=<password>  	Password
	-i <pkey_file> --identity=<pkey_file>  	Private key file [default: ~/.ssh/id_rsa]
	-P <port> --port=<port>  		Port number [default: 22]
	-c <critical> --critical=<critical>		Critical threshold in JSON format example {"cpu":[90,70,50],"free_memory":50,"vpn_users":250,"failover_active":900} 
	-w <warning> --warning=<warning>		Warning threshold in JSON format example {"cpu":[70,50,30],"free_memory":50,"vpn_users":200,"failover_active":1800}`

	// Don't parse command line argument for testing argument must be passed with OS environment variable
	if os.Getenv("CHECK_MODE") == "TEST" {
		params.version, _ = strconv.ParseBool(os.Getenv("VERSION"))
		params.port, _ = strconv.Atoi(os.Getenv("PORT"))
		if params.port == 0 {
			params.port = 22
		}
		params.host = os.Getenv("HOST")
		params.username = os.Getenv("USERNAME")
		params.password = os.Getenv("PASSWORD")
		params.identity = os.Getenv("IDENTITY")
		if params.identity == "" && params.password == "" {
			params.identity = "~/.ssh/id_rsa"
		}
		params.verbose, _ = strconv.ParseBool(os.Getenv("VERBOSE"))
		params.command = os.Getenv("COMMAND")
		params.critical = os.Getenv("CRITICAL")
		params.warning = os.Getenv("WARNING")
	} else {
		arguments, err = docopt.ParseDoc(usage)
		if err != nil {
			fmt.Printf("%s: Error parsing command line arguments: %v", ict.UnkMsg, err)
			os.Exit(ict.UnkExit)
		}

		if c, _ := arguments.Bool("status"); c {
			params.command = "status"
		}
		if c, _ := arguments.Bool("vpnusers"); c {
			params.command = "vpnusers"
		}
		if c, _ := arguments.Bool("failover"); c {
			params.command = "failover"
		}

		params.version, _ = arguments.Bool("--version")
		params.port, _ = arguments.Int("--port")
		params.host, _ = arguments.String("--host")
		params.username, _ = arguments.String("--username")
		params.password, _ = arguments.String("--password")
		params.identity, _ = arguments.String("--identity")
		params.verbose, _ = arguments.Bool("--verbose")
		if params.verbose {
			os.Setenv("VERBOSE", "TRUE")
		}
		params.critical, _ = arguments.String("--critical")
		params.warning, _ = arguments.String("--warning")
	}
}

func main() {
	var err error
	var icinga ict.Icinga
	var asa *CiscoASA

	asa = NewCiscoASA(params.host)

	// We return version of program and exit with Ok status
	if params.version {
		fmt.Printf("check_ciscoswitch version %s-build %s\n", version, buildcount)
		os.Exit(ict.UnkExit)
	}

	// Check command arguments and calling method
	switch params.command {
	case "status":
		icinga, err = asa.CheckStatus(params.host, params.username, params.password, params.identity, params.port, params.critical, params.warning)
		if err != nil {
			fmt.Printf("%s: Error CheckStatus => %s", ict.CriMsg, err)
			os.Exit(ict.CriExit)
		}
		fmt.Println(icinga)
		os.Exit(icinga.Exit)
	case "vpnusers":
		icinga, err = asa.CheckVPNUsers(params.host, params.username, params.password, params.identity, params.port, params.critical, params.warning)
		if err != nil {
			fmt.Printf("%s: Error CheckVPNUsers => %s", ict.CriMsg, err)
			os.Exit(ict.CriExit)
		}

		fmt.Println(icinga)
		os.Exit(icinga.Exit)
	case "failover":
		icinga, err = asa.CheckFailover(params.host, params.username, params.password, params.identity, params.port, params.critical, params.warning)
		if err != nil {
			fmt.Printf("%s: Error CheckFailover => %s", ict.CriMsg, err)
			os.Exit(ict.CriExit)
		}

		fmt.Println(icinga)
		os.Exit(icinga.Exit)
	default:
		fmt.Printf("check_ciscoswitch version %s-build %s\n", version, buildcount)
		fmt.Printf("Usage: %s", usage)
		os.Exit(ict.UnkExit)
	}

}
