#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# The pairing lab's networks: a pretend internet (a bridge in namespace pl-inet, 80.0.0.0/16 and
# 2001:db8::/32, which exist only there), a WittyRelay on it, the server's home (router pl-rs,
# server pl-srv) and the phone's network (router pl-rc, phone pl-cli). Sourced by case.sh.
#
#   lab_net <network> [uplink]     builds it; lab_net_down removes it.
#
# Networks (the server's side / the phone's side / the relay):
#   direct            server on a public address / port-restricted cone / none
#   fullcone          full cone (the P2P port forwarded) / port-restricted cone / own WittyRelay, which
#                     here only introduces them: the way on is direct
#   cone-relay        port-restricted cone / port-restricted cone / own WittyRelay
#   ipv4-only         as cone-relay; the server's IPv6 layer stays on with no IPv6 anywhere
#   symmetric-relay   symmetric / symmetric / own WittyRelay
#   cgnat-relay       cone behind a provider's symmetric CGNAT (100.64.0.0/10) / symmetric / own WittyRelay
#   udp-blocked       port-restricted cone / symmetric, and no UDP out at all (TCP only) / own WittyRelay
#   ipv6-only         IPv6 only: stateful home firewalls, no NAT / own WittyRelay on IPv6
#
# "uplink" also routes the pretend internet out to the real one (NAT through this machine), for
# joining by a code, which needs the public address book (the DHT). Nothing else leaves.
PL_NS="pl-inet pl-relay pl-rs pl-srv pl-rc pl-cli pl-cgn"
PL_RELAY4=80.0.0.10
PL_RELAY6=2001:db8::10
PL_SRV_WAN4=80.0.1.1
PL_CLI_WAN4=80.0.2.1

nsx() { ip netns exec "$@"; }

lab_net_down() {
  local n
  for n in $PL_NS; do ip netns pids "$n" 2>/dev/null | xargs -r kill 2>/dev/null; done
  sleep 0.3
  for n in $PL_NS; do
    ip netns pids "$n" 2>/dev/null | xargs -r kill -9 2>/dev/null
    ip netns del "$n" 2>/dev/null
    rm -rf "/etc/netns/$n"
  done
  ip link del pl-up 2>/dev/null
  iptables -D FORWARD -i pl-up -j ACCEPT 2>/dev/null
  iptables -D FORWARD -o pl-up -j ACCEPT 2>/dev/null
  iptables -t nat -D POSTROUTING -s 80.0.0.0/16 ! -d 80.0.0.0/16 -j MASQUERADE 2>/dev/null
  true
}

_pl_i=0
# _wire NS DEV ADDR...: a cable from NS to the internet's bridge (addresses with their prefix).
_wire() {
  local n=$1 dev=$2 a
  shift 2
  _pl_i=$((_pl_i + 1))
  ip link add "$dev" netns "$n" type veth peer name "p$_pl_i" netns pl-inet
  nsx pl-inet ip link set "p$_pl_i" master br0 up
  for a in "$@"; do nsx "$n" ip addr add "$a" dev "$dev" nodad 2>/dev/null || nsx "$n" ip addr add "$a" dev "$dev"; done
  nsx "$n" ip link set "$dev" up
}

# _home ROUTER HOST NET4 NET6: a cable from a router's lan to one computer behind it.
_home() {
  local r=$1 h=$2 net4=$3 net6=$4
  ip link add lan netns "$r" type veth peer name eth0 netns "$h"
  if [ -n "$net4" ]; then
    nsx "$r" ip addr add "$net4.1/24" dev lan
    nsx "$h" ip addr add "$net4.10/24" dev eth0
  fi
  if [ -n "$net6" ]; then
    nsx "$r" ip -6 addr add "$net6::1/64" dev lan nodad
    nsx "$h" ip -6 addr add "$net6::10/64" dev eth0 nodad
  fi
  nsx "$r" ip link set lan up; nsx "$h" ip link set eth0 up
  [ -n "$net4" ] && nsx "$h" ip route add default via "$net4.1"
  [ -n "$net6" ] && nsx "$h" ip -6 route add default via "$net6::1"
  true
}

# _nat ROUTER MODE: IPv4 NAT on the router's wan. cone: one outside port per inside port, answers
# only from where it sent (port-restricted, what most routers do). fullcone: as cone, and the
# server's P2P port forwarded, so anyone may send to it. symmetric: a new outside port per destination.
_nat() {
  local r=$1 mode=$2 rand=""
  nsx "$r" sysctl -qw net.ipv4.ip_forward=1
  [ "$mode" = symmetric ] && rand="--random-fully"
  # shellcheck disable=SC2086
  nsx "$r" iptables -t nat -A POSTROUTING -o wan -j MASQUERADE $rand
  nsx "$r" iptables -P FORWARD DROP
  nsx "$r" iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  nsx "$r" iptables -A FORWARD -i lan -o wan -j ACCEPT
  if [ "$mode" = fullcone ]; then
    nsx "$r" iptables -t nat -A PREROUTING -i wan -p udp --dport "$PL_P2P" -j DNAT --to-destination "192.168.1.10:$PL_P2P"
    nsx "$r" iptables -t nat -A PREROUTING -i wan -p tcp --dport "$PL_P2P" -j DNAT --to-destination "192.168.1.10:$PL_P2P"
    nsx "$r" iptables -A FORWARD -i wan -d 192.168.1.10 -p udp --dport "$PL_P2P" -j ACCEPT
    nsx "$r" iptables -A FORWARD -i wan -d 192.168.1.10 -p tcp --dport "$PL_P2P" -j ACCEPT
  fi
  # Like a real home router: what arrives uninvited is dropped, not answered with a reset.
  nsx "$r" iptables -A INPUT -i wan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  nsx "$r" iptables -A INPUT -i wan -j DROP
}

# _fw6 ROUTER: an IPv6 home router: no NAT, but nothing in that wasn't asked for.
_fw6() {
  local r=$1
  nsx "$r" sysctl -qw net.ipv6.conf.all.forwarding=1
  nsx "$r" ip6tables -P FORWARD DROP
  nsx "$r" ip6tables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  nsx "$r" ip6tables -A FORWARD -p ipv6-icmp -j ACCEPT
  nsx "$r" ip6tables -A FORWARD -i lan -o wan -j ACCEPT
  nsx "$r" ip6tables -A INPUT -i wan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  nsx "$r" ip6tables -A INPUT -i wan -p ipv6-icmp -j ACCEPT
  nsx "$r" ip6tables -A INPUT -i wan -j DROP
}

# lab_net NETWORK [uplink]: sets PL_RELAY_ADDRS (what the relay announces; empty: no relay) and
# PL_V6 (yes for ipv6-only).
lab_net() {
  local net=$1 uplink=${2:-} n
  lab_net_down
  _pl_i=0
  for n in $PL_NS; do ip netns add "$n"; nsx "$n" ip link set lo up; done
  nsx pl-inet ip link add br0 type bridge
  nsx pl-inet ip link set br0 up
  PL_V6=no
  PL_RELAY_ADDRS=/ip4/$PL_RELAY4/udp/4001/quic-v1,/ip4/$PL_RELAY4/tcp/4001
  local snat=cone cnat=cone
  case "$net" in
    direct) PL_RELAY_ADDRS= ;;
    fullcone) snat=fullcone ;;
    cone-relay | ipv4-only) ;;
    symmetric-relay) snat=symmetric; cnat=symmetric ;;
    cgnat-relay) cnat=symmetric ;;
    udp-blocked) cnat=symmetric ;;
    ipv6-only) PL_V6=yes; PL_RELAY_ADDRS=/ip6/$PL_RELAY6/udp/4001/quic-v1,/ip6/$PL_RELAY6/tcp/4001 ;;
    *) echo "unknown network $net" >&2; return 2 ;;
  esac
  if [ "$PL_V6" = yes ]; then
    nsx pl-inet sysctl -qw net.ipv6.conf.all.disable_ipv6=0
    _wire pl-relay eth0 "$PL_RELAY6/32"
    _wire pl-rs wan 2001:db8:1::1/32
    _wire pl-rc wan 2001:db8:2::1/32
    _home pl-rs pl-srv "" 2001:db8:a
    _home pl-rc pl-cli "" 2001:db8:b
    # Routes to the other home's network go through its router.
    nsx pl-rs ip -6 route add 2001:db8:b::/64 via 2001:db8:2::1
    nsx pl-rc ip -6 route add 2001:db8:a::/64 via 2001:db8:1::1
    nsx pl-relay ip -6 route add 2001:db8:a::/64 via 2001:db8:1::1
    nsx pl-relay ip -6 route add 2001:db8:b::/64 via 2001:db8:2::1
    _fw6 pl-rs; _fw6 pl-rc
    PL_SRV_IP=2001:db8:a::10
  else
    _wire pl-relay eth0 "$PL_RELAY4/16"
    if [ "$net" = direct ]; then
      # The server itself on the internet: its own public address, nothing in front.
      _wire pl-srv eth0 80.0.1.10/16
      PL_SRV_IP=80.0.1.10
    elif [ "$net" = cgnat-relay ]; then
      _wire pl-cgn wan "$PL_SRV_WAN4/16"
      ip link add lan netns pl-cgn type veth peer name wan netns pl-rs
      nsx pl-cgn ip addr add 100.64.0.1/24 dev lan; nsx pl-cgn ip link set lan up
      nsx pl-rs ip addr add 100.64.0.2/24 dev wan; nsx pl-rs ip link set wan up
      nsx pl-rs ip route add default via 100.64.0.1
      _nat pl-cgn symmetric
      _home pl-rs pl-srv 192.168.1 ""
      _nat pl-rs cone
      PL_SRV_IP=192.168.1.10
    else
      _wire pl-rs wan "$PL_SRV_WAN4/16"
      _home pl-rs pl-srv 192.168.1 ""
      _nat pl-rs "$snat"
      PL_SRV_IP=192.168.1.10
    fi
    _wire pl-rc wan "$PL_CLI_WAN4/16"
    _home pl-rc pl-cli 192.168.2 ""
    _nat pl-rc "$cnat"
    # An office or school network: nothing over UDP leaves it.
    [ "$net" = udp-blocked ] && nsx pl-rc iptables -I FORWARD -i lan -p udp -j DROP
    # No IPv6 anywhere: not even link-local chatter.
    for n in pl-srv pl-cli; do nsx "$n" sysctl -qw net.ipv6.conf.all.disable_ipv6=1; done
  fi
  [ "$uplink" = uplink ] && _uplink
  true
}

# The pretend internet's way out to the real one: a cable to this machine, which hands packets on
# with its own address. Every router there gets a default route through it.
_uplink() {
  local r
  nsx pl-inet ip addr add 80.0.0.1/16 dev br0
  ip link add pl-up type veth peer name up netns pl-inet
  ip addr add 10.254.77.1/30 dev pl-up; ip link set pl-up up
  nsx pl-inet ip addr add 10.254.77.2/30 dev up; nsx pl-inet ip link set up up
  nsx pl-inet ip route add default via 10.254.77.1
  nsx pl-inet sysctl -qw net.ipv4.ip_forward=1
  sysctl -qw net.ipv4.ip_forward=1
  ip route add 80.0.0.0/16 via 10.254.77.2 2>/dev/null
  iptables -I FORWARD -i pl-up -j ACCEPT
  iptables -I FORWARD -o pl-up -j ACCEPT
  iptables -t nat -A POSTROUTING -s 80.0.0.0/16 ! -d 80.0.0.0/16 -j MASQUERADE
  for r in pl-rs pl-rc pl-cgn pl-relay pl-srv; do
    nsx "$r" ip route show default 2>/dev/null | grep -q . && continue
    nsx "$r" ip -4 addr show 2>/dev/null | grep -q 'inet 80\.0\.' && nsx "$r" ip route add default via 80.0.0.1
  done
  # Names, for the address book's bootstrap: a resolver the namespaces can reach.
  for r in pl-srv pl-cli; do
    mkdir -p "/etc/netns/$r"
    printf 'nameserver 1.1.1.1\nnameserver 8.8.8.8\n' >"/etc/netns/$r/resolv.conf"
  done
}
