package main

import "os"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "sw-net" {
		SwitchNetworkMain()
	} else {
		SwitchNetworkMain()
	}
}
