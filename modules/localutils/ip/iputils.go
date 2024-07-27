package iputils

import (
	"errors"
	"fmt"
	"net"
)

type Cidr struct {
	Ip         net.IP
	NetAddress *net.IPNet
}

func (c *Cidr) PrefixToMask() (*string, error) {
	m := c.NetAddress.Mask
	if len(m) != 4 {
		return nil, errors.New("Error: IPv4 only.")
	}
	mask := fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
	return &mask, nil
}

func (c *Cidr) GetIpv4WithMask() (*string, error) {
	m := c.NetAddress.Mask
	if len(m) != 4 {
		return nil, errors.New("Error: IPv4 only.")
	}
	mask := fmt.Sprintf("%s %d.%d.%d.%d", c.Ip.String(), m[0], m[1], m[2], m[3])
	return &mask, nil
}

func StringToCidr(s string) (*Cidr, error) {
	ip, netaddr, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	cidr := &Cidr{Ip: ip, NetAddress: netaddr}
	return cidr, nil
}
