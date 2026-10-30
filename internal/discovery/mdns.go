package discovery

import (
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// The DNS side of mDNS (RFC 6762) and DNS-SD (RFC 6763): just enough to be found.
// Questions come from anyone on the network, so parsing is left to dnsmessage.

const (
	ttlLong  = 4500 // names that rarely change
	ttlShort = 120  // what depends on this machine's address
	// flushBit marks a record as the only one of its name (the "cache flush" bit).
	flushBit = dnsmessage.Class(0x8000)
	// unicastBit in a question asks for the answer to come back directly.
	unicastBit = dnsmessage.Class(0x8000)
)

// names are this server's DNS names.
type names struct {
	service, meta, instance, host dnsmessage.Name
}

func namesFor(info Info) (names, error) {
	var n names
	var err error
	if n.service, err = dnsmessage.NewName(ServiceType + ".local."); err != nil {
		return n, err
	}
	if n.meta, err = dnsmessage.NewName("_services._dns-sd._udp.local."); err != nil {
		return n, err
	}
	if n.instance, err = dnsmessage.NewName(instanceLabel(info) + "." + ServiceType + ".local."); err != nil {
		return n, err
	}
	// Not the computer's own name, which may well be a person's.
	if n.host, err = dnsmessage.NewName("wms-" + shortID(info.ID, 8) + ".local."); err != nil {
		return n, err
	}
	return n, nil
}

func shortID(id string, n int) string {
	if len(id) < n {
		return id
	}
	return id[:n]
}

// instanceLabel is the name shown in lists: unique, so two servers with the same
// name don't collide, and with nothing DNS can't hold (dots split labels).
func instanceLabel(info Info) string {
	name := strings.Map(func(r rune) rune {
		if r == '.' || r < 32 || r == 127 {
			return ' '
		}
		return r
	}, info.Name)
	name = strings.TrimSpace(trimBytes(name, 50))
	if name == "" {
		name = "WittyMediaServer"
	}
	return name + " (" + shortID(info.ID, 4) + ")"
}

// trimBytes shortens a string without cutting a character in half.
func trimBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

func txtRecord(info Info) []string {
	tls := "0"
	if info.TLS {
		tls = "1"
	}
	return []string{
		"id=" + info.ID,
		"name=" + trimBytes(info.Name, 200),
		"v=" + trimBytes(info.Version, 40),
		"api=" + strconv.Itoa(info.API),
		"tls=" + tls,
	}
}

func hdr(name dnsmessage.Name, typ dnsmessage.Type, ttl uint32, flush bool) dnsmessage.ResourceHeader {
	class := dnsmessage.ClassINET
	if flush {
		class |= flushBit
	}
	return dnsmessage.ResourceHeader{Name: name, Type: typ, Class: class, TTL: ttl}
}

func (n names) ptr(ttl uint32) dnsmessage.Resource {
	return dnsmessage.Resource{Header: hdr(n.service, dnsmessage.TypePTR, ttl, false), Body: &dnsmessage.PTRResource{PTR: n.instance}}
}

func (n names) srv(info Info, ttl uint32, flush bool) dnsmessage.Resource {
	return dnsmessage.Resource{Header: hdr(n.instance, dnsmessage.TypeSRV, ttl, flush), Body: &dnsmessage.SRVResource{Port: uint16(info.Port), Target: n.host}}
}

func (n names) txt(info Info, ttl uint32, flush bool) dnsmessage.Resource {
	return dnsmessage.Resource{Header: hdr(n.instance, dnsmessage.TypeTXT, ttl, flush), Body: &dnsmessage.TXTResource{TXT: txtRecord(info)}}
}

func (n names) a(ips []net.IP, ttl uint32, flush bool) []dnsmessage.Resource {
	var out []dnsmessage.Resource
	for _, ip := range ips {
		var b [4]byte
		copy(b[:], ip.To4())
		out = append(out, dnsmessage.Resource{Header: hdr(n.host, dnsmessage.TypeA, ttl, flush), Body: &dnsmessage.AResource{A: b}})
	}
	return out
}

func pack(m dnsmessage.Message) []byte {
	b, err := m.Pack()
	if err != nil {
		return nil
	}
	return b
}

// announcement is the unsolicited message saying this server is here, or, with
// a lifetime of zero, that it has gone.
func announcement(info Info, ips []net.IP, goodbye bool) ([]byte, error) {
	n, err := namesFor(info)
	if err != nil {
		return nil, err
	}
	long, short := uint32(ttlLong), uint32(ttlShort)
	if goodbye {
		long, short = 0, 0
	}
	m := dnsmessage.Message{Header: dnsmessage.Header{Response: true, Authoritative: true}}
	m.Answers = append(m.Answers, n.ptr(long), n.srv(info, short, true), n.txt(info, long, true))
	m.Answers = append(m.Answers, n.a(ips, short, true)...)
	return pack(m), nil
}

func sameName(a, b dnsmessage.Name) bool { return strings.EqualFold(a.String(), b.String()) }

// answer replies to one packet, if it asks about this server. unicast says the
// reply goes straight back to the asker rather than to the group: when it asked
// for that, or when it isn't a full mDNS client (its port isn't 5353), which the
// RFC calls a legacy query and answers in plain DNS style.
func answer(packet []byte, srcPort int, info Info, ips []net.IP) (resp []byte, unicast bool) {
	var p dnsmessage.Parser
	h, err := p.Start(packet)
	if err != nil || h.Response || h.OpCode != 0 {
		return nil, false
	}
	qs, err := p.AllQuestions()
	if err != nil {
		return nil, false
	}
	n, err := namesFor(info)
	if err != nil {
		return nil, false
	}
	legacy := srcPort != mdnsPort
	flush := !legacy
	long, short := uint32(ttlLong), uint32(ttlShort)
	if legacy { // the RFC's limit, since such a client can't be told of changes
		long, short = 10, 10
	}
	var answers, extra []dnsmessage.Resource
	var asked []dnsmessage.Question
	seen := map[dnsmessage.Type]bool{}
	add := func(r ...dnsmessage.Resource) {
		for _, x := range r {
			if !seen[x.Header.Type] {
				answers = append(answers, x)
			}
			seen[x.Header.Type] = true
		}
	}
	wantsDirect := len(qs) > 0
	for _, q := range qs {
		class := q.Class &^ unicastBit
		if class != dnsmessage.ClassINET && class != dnsmessage.ClassANY {
			continue
		}
		hit := true
		switch {
		case sameName(q.Name, n.meta) && (q.Type == dnsmessage.TypePTR || q.Type == dnsmessage.TypeALL):
			answers = append(answers, dnsmessage.Resource{Header: hdr(n.meta, dnsmessage.TypePTR, long, false), Body: &dnsmessage.PTRResource{PTR: n.service}})
		case sameName(q.Name, n.service) && (q.Type == dnsmessage.TypePTR || q.Type == dnsmessage.TypeALL):
			add(n.ptr(long))
			extra = append(extra, n.srv(info, short, flush), n.txt(info, long, flush))
			extra = append(extra, n.a(ips, short, flush)...)
		case sameName(q.Name, n.instance) && (q.Type == dnsmessage.TypeSRV || q.Type == dnsmessage.TypeALL):
			add(n.srv(info, short, flush))
			extra = append(extra, n.a(ips, short, flush)...)
		case sameName(q.Name, n.instance) && q.Type == dnsmessage.TypeTXT:
			add(n.txt(info, long, flush))
		case sameName(q.Name, n.host) && (q.Type == dnsmessage.TypeA || q.Type == dnsmessage.TypeALL):
			answers = append(answers, n.a(ips, short, flush)...)
		default:
			hit = false
		}
		if hit {
			asked = append(asked, dnsmessage.Question{Name: q.Name, Type: q.Type, Class: class})
			if q.Class&unicastBit == 0 {
				wantsDirect = false
			}
		}
	}
	if len(answers) == 0 {
		return nil, false
	}
	m := dnsmessage.Message{Header: dnsmessage.Header{Response: true, Authoritative: true}, Answers: answers, Additionals: extra}
	if legacy {
		m.Header.ID = h.ID
		m.Questions = asked
	}
	return pack(m), legacy || wantsDirect
}
