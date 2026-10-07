#!/bin/sh
# Records the command outputs the bot relies on, in the fixture format of
# internal/repo/execx/testdata/router.txt, with MAC and public addresses
# redacted. Run it ON the router and paste the output into a fixture file:
#
#   ssh root@192.168.1.1 'sh -s' < scripts/fixtures.sh > internal/repo/execx/testdata/myrouter.txt
#
redact() { sed -E 's/([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}/aa:bb:cc:dd:ee:ff/g; s/"(private_key|key|password|token)": *"[^"]*"/"\1": "***"/g'; }
block() { # name command...
	echo ">>> $1"; shift
	"$@" 2>/dev/null | redact
}
block "ubus call system board" ubus call system board
block "ubus call system info" ubus call system info
block "ubus call network.interface dump" ubus call network.interface dump
block "ubus list hostapd.*" ubus list 'hostapd.*'
for o in $(ubus list 'hostapd.*'); do block "ubus call $o get_clients" ubus call "$o" get_clients; done
block "ubus call luci-rpc getHostHints" ubus call luci-rpc getHostHints
block "ubus call service list" ubus call service list
block "ubus call uci get {\"config\":\"dhcp\"}" ubus call uci get '{"config":"dhcp"}'
for i in $(ubus call network.interface dump | jsonfilter -e '@.interface[@.proto="wireguard" || @.proto="amneziawg"].interface'); do
	block "ubus call network.interface.$i status" ubus call "network.interface.$i" status
	tool=wg; command -v awg >/dev/null && tool=awg
	echo ">>> $tool show $i dump"
	$tool show "$i" dump 2>/dev/null | awk 'NR==1 {$1="(hidden)"} {print}' | redact
done
block "ip route show 0.0.0.0/1" ip route show 0.0.0.0/1
block "ip rule show pref 29990" ip rule show pref 29990
for p in awg wg sysupgrade logread ip; do command -v $p >/dev/null && echo ">>>path $p 1" || echo ">>>path $p 0"; done
