// This programm collects some system information, formats it nicely and sets
// the X root windows name so it can be displayed in the dwm status bar.
//
// The strange characters in the output are used by dwm to colorize the output
// ( to , needs the http://dwm.suckless.org/patches/statuscolors patch) and
// as Icons or separators (e.g. "Ý"). If you don't use the status-18 font
// (https://github.com/schachmat/status-18), you should probably exchange them
// by something else ("CPU", "MEM", "|" for separators, …).
//
// For license information see the file LICENSE
package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"regexp"
)

const (
	bpsSign   = "b"
	kibpsSign = "K"
	mibpsSign = "M"

	batterySign100 = ""
	batterySign75 = ""
	batterySign50 = ""
	batterySign25 = ""
	batterySign10 = ""
	pluggedSign   = ""

	cpuSign = ""
	cpuTempSign = ""
	memSign = ""

	netReceivedSign    = "⮮"
	netTransmittedSign = "⮭"
	pingSign = "⭿"

	volSign = ""
	mutedSign = ""

	wifiSignFull = "⡆"
	wifiSignHalf = "⡄"
	wifiSignLow = "⡀"
	wifiSignOff = "⨯"

	keyboardSign = ""

	floatSeparator = "."
	dateSeparator  = ""
	fieldSeparator = " "
)

var (
	netDevs = map[string]struct{}{
		"eth0:": {},
		"eth1:": {},
		"wlan0:": {},
		"ppp0:": {},
		"wlp4s0:": {},
	}
	cores = runtime.NumCPU() // count of cores to scale cpu usage
	rxOld = 0
	txOld = 0
)

// fixed builds a fixed width string with given pre- and fitting suffix
func fixed(pre string, rate int) string {
	if rate < 0 {
		return pre + " ERR"
	}

	var decDigit = 0
	var suf = bpsSign // default: display as B/s

	switch {
	case rate >= (1000 * 1024 * 1024): // > 999 MiB/s
		return pre + " ERR"
	case rate >= (1000 * 1024): // display as MiB/s
		decDigit = (rate / 1024 / 102) % 10
		rate /= (1024 * 1024)
		suf = mibpsSign
	case rate >= 1000: // display as KiB/s
		decDigit = (rate / 102) % 10
		rate /= 1024
		suf = kibpsSign
	}

	var formated = ""
	if rate >= 100 {
		formated = fmt.Sprintf(" %3d", rate)
	} else if rate >= 10 {
		formated = fmt.Sprintf("%2d.%1d", rate, decDigit)
	} else {
		formated = fmt.Sprintf(" %1d.%1d", rate, decDigit)
	}
	return pre + strings.Replace(formated, ".", floatSeparator, 1) + suf
}

// updateNetUse reads current transfer rates of certain network interfaces
func updateNetUse() string {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return netReceivedSign + " ERR " + netTransmittedSign + " ERR"
	}
	defer file.Close()

	var void = 0 // target for unused values
	var dev, rx, tx, rxNow, txNow = "", 0, 0, 0, 0
	var scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		_, err = fmt.Sscanf(scanner.Text(), "%s %d %d %d %d %d %d %d %d %d",
			&dev, &rx, &void, &void, &void, &void, &void, &void, &void, &tx)
		if _, ok := netDevs[dev]; ok {
			rxNow += rx
			txNow += tx
		}
	}

	// attempt to read avgping file
	// add the following to your crontab:
	// */1 * * * * ping -c 4 www.google.com -s 16 | tail -1| awk '{print $4}' | cut -d '/' -f 2 > /home/john/tmp/avgping2 && mv /home/john/tmp/avgping2 /home/john/tmp/avgping
	var avgping, err2 = ioutil.ReadFile("/home/john/tmp/avgping")
	var ping, pingAvg = "", 0.0
	if err2 != nil {
		ping = ""
	} else {
		_, err = fmt.Sscanf(string(avgping), "%f", &pingAvg)
		if err != nil {
			ping = " " + pingSign + "0.0ms"
		} else {
			ping = fmt.Sprintf(" %s %dms", pingSign, int(pingAvg))
		}
	}

	defer func() { rxOld, txOld = rxNow, txNow }()
	return fmt.Sprintf("%s %s%s", fixed(netReceivedSign, rxNow-rxOld), fixed(netTransmittedSign, txNow-txOld), ping)
}

// colored surrounds the percentage with color escapes if it is >= 70
func colored(icon string, percentage int) string {
	if percentage >= 100 {
		return fmt.Sprintf("%s%3d", icon, percentage)
	} else if percentage >= 70 {
		return fmt.Sprintf("%s%3d", icon, percentage)
	}
	return fmt.Sprintf("%s%3d", icon, percentage)
}

// updatePower reads the current battery and power plug status
func updatePower() string {
	const powerSupply = "/sys/class/power_supply/"
	var enFull, enNow, enPerc int = 0, 0, 0
	var plugged, err = ioutil.ReadFile(powerSupply + "AC/online")
	if err != nil {
		return "|ERR"
	}
	batts, err := ioutil.ReadDir(powerSupply)
	if err != nil {
		return "|ERR"
	}

	readval := func(name, field string) int {
		var path = powerSupply + name + "/"
		var file []byte
		if tmp, err := ioutil.ReadFile(path + "energy_" + field); err == nil {
			file = tmp
		} else if tmp, err := ioutil.ReadFile(path + "charge_" + field); err == nil {
			file = tmp
		} else {
			return 0
		}

		if ret, err := strconv.Atoi(strings.TrimSpace(string(file))); err == nil {
			return ret
		}
		return 0
	}

	for _, batt := range batts {
		name := batt.Name()
		if !strings.HasPrefix(name, "BAT") {
			continue
		}

		enFull += readval(name, "full")
		enNow += readval(name, "now")
	}

	if enFull == 0 { // Battery found but no readable full file.
		return "|ERR"
	}

	enPerc = enNow * 100 / enFull
	var icon = batterySign100
	var icon2 = ""
	if string(plugged) == "1\n" {
		icon = pluggedSign
		if enPerc <= 98 {
			icon2 = ""
		}
	} else if enPerc <= 10 {
		icon = batterySign10
	} else if enPerc <= 25 {
		icon = batterySign25
	} else if enPerc <= 50 {
		icon = batterySign50
	} else if enPerc <= 75 {
		icon = batterySign75
	} else if enPerc <= 100 {
		icon = batterySign100
		icon2 = ""
	}
	return fmt.Sprintf("%s%s%3d%%", icon, icon2, enPerc)
}

// updatePowerTime runs acpi -b to get the time to deplete/full charge the battery
func updatePowerTime() string {
	var out, err = exec.Command("acpi", "-b").Output()
	if err != nil {
		return "unknown"
	}
	acpi := string(out)
	timeRx := regexp.MustCompile(`.*(\d\d:\d\d:\d\d).*`)
	acpiMatch := timeRx.FindStringSubmatch(acpi)
	if len(acpiMatch) == 1 {
		return "unknown"
	} else {
		return acpiMatch[1][0:5]
	}
}

// updateCPUUse reads the last minute sysload and scales it to the core count
func updateCPUUse() string {
	var load float32
	var loadavg, err = ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return cpuSign + "ERR"
	}
	_, err = fmt.Sscanf(string(loadavg), "%f", &load)
	if err != nil {
		return cpuSign + "ERR"
	}
	return fmt.Sprintf("%s%3d%%", cpuSign, int(load*100.0/float32(cores)))
}

// updateMemUse reads the memory used by applications and scales to [0, 100]
func updateMemUse() string {
	var file, err = os.Open("/proc/meminfo")
	if err != nil {
		return memSign + "ERR"
	}
	defer file.Close()

	// done must equal the flag combination (0001 | 0010 | 0100 | 1000) = 15
	var used, total, done = 0.0, 0.0, 0
	for info := bufio.NewScanner(file); done != 15 && info.Scan(); {
		var prop, val = "", 0.0
		if _, err = fmt.Sscanf(info.Text(), "%s %f", &prop, &val); err != nil {
			return memSign + "ERR"
		}
		switch prop {
		case "MemTotal:":
			total = val
			used += val
			done |= 1
		case "MemFree:":
			used -= val
			done |= 2
		case "Buffers:":
			used -= val
			done |= 4
		case "Cached:":
			used -= val
			done |= 8
		}
	}
	used = used / 1024 / 1024
	total = total / 1024 / 1024
	return fmt.Sprintf("%s %.2f/%.2fGB", memSign, used, total)
}

func updateVolume() string {
	var out, err = exec.Command("pacmd", "list-sinks").Output()
	if err != nil {
		return mutedSign + " ERR"
	}
	var sign = volSign
	pacmd := string(out)
	mutedRx := regexp.MustCompile(`(?s).*volume: front-left: .* (\d*%) /.*front-right: .* (\d*%).*muted: (yes|no).*`)
	pacmdMatch := mutedRx.FindStringSubmatch(pacmd)
	if pacmdMatch[3] == "yes" {
		sign = mutedSign
	}
	return sign + " " + pacmdMatch[1]
}

func updateWifi() string {
	var out, err = exec.Command("awk", "NR==3 {printf \"%3.0f\" ,($3/70)*100}", "/proc/net/wireless").Output()
	if err != nil {
		return wifiSignOff + " ERR"
	}
	strength := strings.Trim(string(out), " ")
	if strength != "" {
		strengthInt, err := strconv.Atoi(strength)
		if err != nil {
			return wifiSignOff + " ERR"
		}
		var wifiSign = wifiSignFull
		if strengthInt > 70 {
			wifiSign = wifiSignFull
		} else if strengthInt > 50 {
			wifiSign = wifiSignHalf
		} else if strengthInt > 20 {
			wifiSign = wifiSignLow
		} else {
			wifiSign = wifiSignOff
		}
		if strengthInt >= 100 {
			return wifiSign + "" + strength + "%"
		} else if strengthInt >= 10 {
			return wifiSign + " " + strength + "%"
		} else {
			return wifiSign + "  " + strength + "%"
		}
	} else {
		return wifiSignOff + " 0%"
	}
}

func updateCPUTemp() string {
	var file, err = os.Open("/sys/class/thermal/thermal_zone1/temp")
	if err != nil {
		return cpuTempSign + " ERR"
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var tempStr = " ERR"
	for scanner.Scan() {
		tempStr = scanner.Text()
	}
	temp, err := strconv.Atoi(tempStr)
	if err != nil {
		return cpuTempSign + " ERR"
	}
	temp = temp / 1000
	return fmt.Sprintf("%s %d°C", cpuTempSign, temp)
}

func updateKeyboard() string {
	var file, err = os.Open("/home/john/.config/xmodmap_switcher/state")
	if err != nil {
		return keyboardSign + " default"
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var keyboard = "default"
	for scanner.Scan() {
		keyboard = scanner.Text()
	}
	return keyboardSign + " " + keyboard
}

func getDistroSign() string {
	var out, err = exec.Command("uname", "-a").Output()
	if err != nil {
		return ""
	}
	uname := string(out)
	distroRx := regexp.MustCompile(`.*(arch|slack).*`)
	distroMatch := distroRx.FindStringSubmatch(uname)
	if len(distroMatch) == 1 {
		return ""
	} else if distroMatch[1] == "arch" {
		return ""
	} else if distroMatch[1] == "slack" {
		return ""
	} else {
		return ""
	}
}

// main updates the dwm statusbar every second
func main() {
	distroSign := getDistroSign()
	for {
		var status = []string{
			"",
			updateVolume(),
			updateWifi(),
			updateNetUse(),
			updateCPUUse(),
			updateCPUTemp(),
			updateMemUse(),
			updatePower(),
			updatePowerTime(),
			time.Now().Local().Format(dateSeparator + " Mon Jan 02 15:04"),
			updateKeyboard(),
			distroSign,
		}
		exec.Command("xsetroot", "-name", strings.Join(status, fieldSeparator)).Run()

		// sleep until beginning of next second
		var now = time.Now()
		time.Sleep(now.Truncate(time.Second).Add(time.Second).Sub(now))
		// time.Sleep(5)
	}
}
