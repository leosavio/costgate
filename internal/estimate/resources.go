package estimate

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/leosavio/costgate/internal/catalog"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/usage"
)

// comp is one priced line before the unit price is looked up.
type comp struct {
	name     string
	key      string
	qty      *big.Rat
	unit     string
	factor   *big.Rat // nil means 1
	premise  string   // usage field when the quantity is a premise
	declared bool     // false: premise missing, quantity is 0
	note     string
}

// side is the estimate of one side of a change.
type side struct {
	comps    []comp
	unpriced string
}

type estimator func(a attrs, u usage.Resolved) side

func unpriced(format string, args ...any) side {
	return side{unpriced: fmt.Sprintf(format, args...)}
}

var (
	keyNATHours      = catalog.Key("AmazonEC2", "NatGateway-Hours", "NatGateway")
	keyNATBytes      = catalog.Key("AmazonEC2", "NatGateway-Bytes", "NatGateway")
	keyIPv4          = catalog.Key("AmazonVPC", "PublicIPv4:InUseAddress", "")
	keyEndpointHours = catalog.Key("AmazonVPC", "VpcEndpoint-Hours", "VpcEndpoint")
	keyEndpointBytes = catalog.Key("AmazonVPC", "VpcEndpoint-Bytes", "VpcEndpoint")
	keyTGWVPCHours   = catalog.Key("AmazonVPC", "TransitGateway-Hours", "TransitGatewayVPC")
	keyTGWVPCBytes   = catalog.Key("AmazonVPC", "TransitGateway-Bytes", "TransitGatewayVPC")
	keyTGWPeerHours  = catalog.Key("AmazonVPC", "TransitGateway-Hours", "TransitGatewayPeering")
	keyTGWVPNHours   = catalog.Key("AmazonVPC", "TransitGateway-Hours", "TransitGatewayVPN")
	keyTGWVPNBytes   = catalog.Key("AmazonVPC", "TransitGateway-Bytes", "TransitGatewayVPN")
	keyVPNHours      = catalog.Key("AmazonVPC", "VPN-Usage-Hours:ipsec.1", "CreateVpnConnection")
	keyHostedZone    = catalog.Key("AmazonRoute53", "HostedZone", "")
	keyResolverENI   = catalog.Key("AmazonRoute53", "ResolverNetworkInterface", "")
	keyKMSKey        = catalog.Key("awskms", "KMS-Keys", "")
	keyS3Standard    = catalog.Key("AmazonS3", "TimedStorage-ByteHrs", "")
	keyTrailEvents   = catalog.Key("AWSCloudTrail", "PaidEventsRecorded", "")
	keyPCAGeneral    = catalog.Key("AWSCertificateManager", "PaidPrivateCA", "")
	keyPCAShortLived = catalog.Key("AWSCertificateManager", "ShortLivedCertificatePrivateCA", "")
)

func instanceKey(instanceType string) string {
	return catalog.Key("AmazonEC2", "BoxUsage:"+instanceType, "RunInstances")
}

// ebsUsageTypes maps priced EBS volume types to their usage type. io1/io2
// carry provisioned IOPS charges and are not modeled.
var ebsUsageTypes = map[string]string{
	"standard": "EBS:VolumeUsage",
	"gp2":      "EBS:VolumeUsage.gp2",
	"gp3":      "EBS:VolumeUsage.gp3",
	"st1":      "EBS:VolumeUsage.st1",
	"sc1":      "EBS:VolumeUsage.sc1",
}

// estimators maps resource types to their cost formula.
var estimators = map[string]estimator{
	"aws_nat_gateway":                            natGateway,
	"aws_eip":                                    eip,
	"aws_instance":                               instance(false),
	"aws_spot_instance_request":                  instance(true),
	"aws_ebs_volume":                             ebsVolume,
	"aws_vpc_endpoint":                           vpcEndpoint,
	"aws_ec2_transit_gateway_vpc_attachment":     tgwVPCAttachment,
	"aws_ec2_transit_gateway_peering_attachment": tgwPeeringAttachment,
	"aws_vpn_connection":                         vpnConnection,
	"aws_route53_zone":                           monthly("hosted zone", keyHostedZone, "zone-month", "first-25-zones tier"),
	"aws_route53_resolver_endpoint":              resolverEndpoint,
	"aws_kms_key":                                monthly("customer managed key", keyKMSKey, "key-month", "requests not priced"),
	"aws_kms_replica_key":                        monthly("customer managed key (replica)", keyKMSKey, "key-month", "requests not priced"),
	"aws_kms_external_key":                       monthly("customer managed key (external)", keyKMSKey, "key-month", "requests not priced"),
	"aws_kms_replica_external_key":               monthly("customer managed key (external replica)", keyKMSKey, "key-month", "requests not priced"),
	"aws_s3_bucket":                              s3Bucket,
	"aws_cloudtrail":                             cloudTrail,
	"aws_acmpca_certificate_authority":           privateCA,
	"aws_acm_certificate":                        acmCertificate,
	"aws_accessanalyzer_analyzer":                accessAnalyzer,
}

// zeroCostTypes have no charge of their own. Their indirect costs (traffic,
// queries) are out of scope.
var zeroCostTypes = map[string]bool{
	"aws_vpc": true, "aws_subnet": true, "aws_internet_gateway": true, "aws_internet_gateway_attachment": true,
	"aws_egress_only_internet_gateway": true, "aws_route": true, "aws_main_route_table_association": true,
	"aws_default_route_table": true, "aws_default_network_acl": true, "aws_default_security_group": true,
	"aws_default_vpc_dhcp_options": true, "aws_security_group": true, "aws_security_group_rule": true,
	"aws_eip_association": true, "aws_customer_gateway": true, "aws_vpn_gateway": true,
	"aws_vpn_gateway_attachment": true, "aws_vpn_gateway_route_propagation": true, "aws_vpn_connection_route": true,
	"aws_ec2_transit_gateway": true, "aws_ec2_transit_gateway_vpc_attachment_accepter": true,
	"aws_ec2_transit_gateway_peering_attachment_accepter": true, "aws_vpc_endpoint_route_table_association": true,
	"aws_route53_record": true, "aws_route53_zone_association": true, "aws_route53_vpc_association_authorization": true,
	"aws_route53_resolver_rule": true, "aws_route53_resolver_rule_association": true,
	"aws_kms_alias": true, "aws_kms_grant": true, "aws_kms_key_policy": true, "aws_launch_template": true,
	"aws_acm_certificate_validation": true, "aws_budgets_budget": true,
	"aws_cloudtrail_organization_delegated_admin_account": true, "aws_account_alternate_contact": true,
	"aws_account_primary_contact": true, "aws_ebs_encryption_by_default": true, "aws_ebs_default_kms_key": true,
	"aws_s3_account_public_access_block": true, "aws_ec2_instance_metadata_defaults": true,
	"aws_ec2_serial_console_access": true, "aws_ebs_snapshot_block_public_access": true,
	"aws_ec2_image_block_public_access": true, "aws_vpc_block_public_access_options": true,
	"aws_vpc_block_public_access_exclusion": true,
	"aws_s3_bucket_policy":                  true, "aws_s3_bucket_versioning": true, "aws_s3_bucket_public_access_block": true,
	"aws_s3_bucket_server_side_encryption_configuration": true, "aws_s3_bucket_ownership_controls": true,
	"aws_s3_bucket_acl": true, "aws_s3_bucket_lifecycle_configuration": true, "aws_s3_bucket_cors_configuration": true,
	"aws_s3_bucket_logging": true, "aws_s3_bucket_notification": true, "aws_s3_bucket_object_lock_configuration": true,
	"terraform_data": true, "null_resource": true,
}

var zeroCostPrefixes = []string{
	"aws_organizations_", "aws_ssoadmin_", "aws_identitystore_", "aws_iam_", "aws_ram_",
	"aws_vpc_security_group_", "aws_route_table", "aws_network_acl", "aws_vpc_dhcp_options",
	"aws_vpc_peering_connection", "aws_ec2_transit_gateway_route",
	"random_", "time_", "tls_", "local_",
}

func isZeroCost(t string) bool {
	if zeroCostTypes[t] {
		return true
	}
	for _, p := range zeroCostPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func hourly(name, key string, u usage.Resolved, mult *big.Rat) comp {
	q := u.Hours.V
	if mult != nil {
		q = money.Mul(q, mult)
	}
	return comp{name: name, key: key, qty: q, unit: "h", declared: true}
}

func gbComp(name, key string, u usage.Resolved) comp {
	return comp{name: name, key: key, qty: u.GB.V, unit: "GB", premise: "gb_per_month", declared: u.GB.Declared}
}

func monthly(name, key, unit, note string) estimator {
	return func(attrs, usage.Resolved) side {
		return side{comps: []comp{{name: name, key: key, qty: money.Int(1), unit: unit, declared: true, note: note}}}
	}
}

func natGateway(a attrs, u usage.Resolved) side {
	if m, _ := a.str("availability_mode"); m == "regional" {
		return unpriced("regional NAT gateway (availability_mode = regional) is not modeled")
	}
	return side{comps: []comp{
		hourly("NAT gateway hours", keyNATHours, u, nil),
		gbComp("NAT data processed", keyNATBytes, u),
	}}
}

func eip(a attrs, u usage.Resolved) side {
	return side{comps: []comp{hourly("public IPv4 address", keyIPv4, u, nil)}}
}

func volumeComp(name, volType string, size *big.Rat, iops, throughput *big.Rat) (comp, string) {
	ut, ok := ebsUsageTypes[volType]
	if !ok {
		return comp{}, fmt.Sprintf("EBS volume type %q is not modeled (provisioned IOPS)", volType)
	}
	if volType == "gp3" {
		if iops != nil && iops.Cmp(money.Int(3000)) > 0 {
			return comp{}, "gp3 IOPS above the 3000 baseline are not modeled"
		}
		if throughput != nil && throughput.Cmp(money.Int(125)) > 0 {
			return comp{}, "gp3 throughput above the 125 MiB/s baseline is not modeled"
		}
	}
	return comp{name: name + " (" + volType + ")", key: catalog.Key("AmazonEC2", ut, ""), qty: size, unit: "GB-month", declared: true}, ""
}

func instance(forceSpot bool) estimator {
	return func(a attrs, u usage.Resolved) side {
		it, ok := a.str("instance_type")
		if !ok || it == "" {
			return unpriced("instance_type unknown at plan time")
		}
		if t, ok := a.str("tenancy"); ok && t != "" && t != "default" {
			return unpriced("tenancy %q is not modeled (only shared tenancy)", t)
		}
		spot := forceSpot
		if mo, ok := a.first("instance_market_options"); ok {
			if mt, _ := mo.str("market_type"); mt == "spot" {
				spot = true
			}
		}
		var s side
		c := hourly("compute on-demand, Linux "+it, instanceKey(it), u, nil)
		if spot {
			c.name = "compute Spot, Linux " + it
			c.factor = money.Sub(money.Int(1), u.SpotDiscount.V)
			c.premise = "spot_discount"
			c.declared = true
			c.note = "on-demand × (1 − spot_discount " + money.Trim(u.SpotDiscount.V) + ")"
		}
		s.comps = append(s.comps, c)
		if pub, _ := a.boolean("associate_public_ip_address"); pub {
			s.comps = append(s.comps, hourly("public IPv4 address", keyIPv4, u, nil))
		}

		root, hasRoot := a.first("root_block_device")
		size, sizeOK := root.number("volume_size")
		volType, typeOK := root.str("volume_type")
		note := ""
		if !hasRoot || !sizeOK {
			size = u.RootVolumeGB.V
			note = "root size unknown at plan time; root_volume_gb premise " + money.Trim(size) + " GB (" + u.RootVolumeGB.From + ")"
		}
		if !typeOK || volType == "" {
			volType = "gp2"
			if note != "" {
				note += "; "
			}
			note += "root type unknown; assumed gp2"
		}
		iops, _ := root.number("iops")
		tp, _ := root.number("throughput")
		rc, why := volumeComp("root volume", volType, size, iops, tp)
		if why != "" {
			return unpriced("root_block_device: %s", why)
		}
		rc.note = note
		s.comps = append(s.comps, rc)

		devices, _ := a.list("ebs_block_device")
		for i, d := range devices {
			da := newAttrs(d, nil)
			sz, ok := da.number("volume_size")
			if !ok {
				return unpriced("ebs_block_device[%d]: volume_size unknown at plan time", i)
			}
			vt, ok := da.str("volume_type")
			if !ok || vt == "" {
				vt = "gp2"
			}
			io, _ := da.number("iops")
			th, _ := da.number("throughput")
			dc, why := volumeComp(fmt.Sprintf("ebs_block_device[%d]", i), vt, sz, io, th)
			if why != "" {
				return unpriced("ebs_block_device[%d]: %s", i, why)
			}
			s.comps = append(s.comps, dc)
		}
		return s
	}
}

func ebsVolume(a attrs, u usage.Resolved) side {
	size, ok := a.number("size")
	if !ok {
		return unpriced("size unknown at plan time")
	}
	volType, ok := a.str("type")
	note := ""
	if !ok || volType == "" {
		volType, note = "gp2", "type unknown; assumed gp2"
	}
	iops, _ := a.number("iops")
	tp, _ := a.number("throughput")
	c, why := volumeComp("EBS volume", volType, size, iops, tp)
	if why != "" {
		return unpriced("%s", why)
	}
	c.note = note
	return side{comps: []comp{c}}
}

func vpcEndpoint(a attrs, u usage.Resolved) side {
	t, ok := a.str("vpc_endpoint_type")
	if !ok || t == "" {
		t = "Gateway"
	}
	switch t {
	case "Gateway":
		return side{}
	case "Interface":
	default:
		return unpriced("vpc_endpoint_type %q is not modeled", t)
	}
	azs := u.AZCount.V
	note := ""
	if l, ok := a.list("subnet_ids"); ok && len(l) > 0 && !a.wholeUnknown("subnet_ids") {
		azs = money.Int(int64(len(l)))
	} else {
		note = "subnet count unknown at plan time; az_count premise " + money.Trim(azs) + " (" + u.AZCount.From + ")"
	}
	h := hourly("endpoint hours × "+money.Trim(azs)+" AZ", keyEndpointHours, u, azs)
	h.note = note
	return side{comps: []comp{h, gbComp("endpoint data processed", keyEndpointBytes, u)}}
}

func tgwVPCAttachment(a attrs, u usage.Resolved) side {
	return side{comps: []comp{
		hourly("TGW VPC attachment hours", keyTGWVPCHours, u, nil),
		gbComp("TGW data processed", keyTGWVPCBytes, u),
	}}
}

func tgwPeeringAttachment(a attrs, u usage.Resolved) side {
	return side{comps: []comp{hourly("TGW peering attachment hours", keyTGWPeerHours, u, nil)}}
}

func vpnConnection(a attrs, u usage.Resolved) side {
	if bw, _ := a.str("tunnel_bandwidth"); bw == "large" {
		return unpriced("large-bandwidth VPN tunnels are not modeled")
	}
	s := side{comps: []comp{hourly("VPN connection hours", keyVPNHours, u, nil)}}
	tgw, _ := a.str("transit_gateway_id")
	if tgw != "" || a.isUnknown("transit_gateway_id") {
		s.comps = append(s.comps,
			hourly("TGW VPN attachment hours", keyTGWVPNHours, u, nil),
			gbComp("TGW VPN data processed", keyTGWVPNBytes, u))
	}
	return s
}

func resolverEndpoint(a attrs, u usage.Resolved) side {
	n := int64(2)
	note := "ENI count unknown; minimum of 2 assumed"
	if l, ok := a.list("ip_address"); ok && len(l) > 0 {
		note = ""
		n = int64(len(l))
		if n < 2 {
			n, note = 2, "AWS bills a minimum of 2 ENIs"
		}
	}
	c := hourly(fmt.Sprintf("Resolver ENI hours × %d", n), keyResolverENI, u, money.Int(n))
	c.note = note
	return side{comps: []comp{c}}
}

func s3Bucket(a attrs, u usage.Resolved) side {
	c := comp{name: "S3 Standard storage", key: keyS3Standard, qty: u.GB.V, unit: "GB-month",
		premise: "gb_per_month", declared: u.GB.Declared, note: "first 50 TB tier; requests not priced"}
	return side{comps: []comp{c}}
}

func cloudTrail(a attrs, u usage.Resolved) side {
	selectors, _ := a.list("event_selector")
	for _, sel := range selectors {
		if l, ok := newAttrs(sel, nil).list("data_resource"); ok && len(l) > 0 {
			return unpriced("data events (event_selector.data_resource) are not modeled")
		}
	}
	advanced, _ := a.list("advanced_event_selector")
	for _, sel := range advanced {
		if !managementOnly(newAttrs(sel, nil)) {
			return unpriced("advanced_event_selector with data or network events is not modeled")
		}
	}
	if l, ok := a.list("insight_selector"); ok && len(l) > 0 {
		return unpriced("CloudTrail Insights is not modeled")
	}
	return side{comps: []comp{{name: "paid events recorded", key: keyTrailEvents, qty: u.Events.V, unit: "events",
		premise: "events_per_month", declared: u.Events.Declared,
		note: "first copy of management events is free; declare only paid events"}}}
}

func managementOnly(sel attrs) bool {
	fields, _ := sel.list("field_selector")
	for _, f := range fields {
		fa := newAttrs(f, nil)
		if name, _ := fa.str("field"); name != "eventCategory" {
			continue
		}
		eq, _ := fa.list("equals")
		if len(eq) == 0 {
			return false
		}
		for _, v := range eq {
			if s, _ := v.(string); s != "Management" {
				return false
			}
		}
		return true
	}
	return false
}

func privateCA(a attrs, u usage.Resolved) side {
	mode, _ := a.str("usage_mode")
	if mode == "SHORT_LIVED_CERTIFICATE" {
		return side{comps: []comp{{name: "Private CA (short-lived mode)", key: keyPCAShortLived, qty: money.Int(1), unit: "CA-month", declared: true, note: "certificates not priced"}}}
	}
	return side{comps: []comp{{name: "Private CA (general-purpose mode)", key: keyPCAGeneral, qty: money.Int(1), unit: "CA-month", declared: true, note: "certificates not priced"}}}
}

func acmCertificate(a attrs, u usage.Resolved) side {
	if ca, _ := a.str("certificate_authority_arn"); ca != "" || a.isUnknown("certificate_authority_arn") {
		return unpriced("private certificate issued by AWS Private CA is not modeled")
	}
	if opt, ok := a.first("options"); ok {
		if exp, _ := opt.str("export"); exp == "ENABLED" {
			return unpriced("exportable public certificate is not modeled")
		}
	}
	return side{}
}

func accessAnalyzer(a attrs, u usage.Resolved) side {
	t, _ := a.str("type")
	switch t {
	case "", "ACCOUNT", "ORGANIZATION":
		return side{}
	}
	return unpriced("analyzer type %q (unused or internal access) is not modeled", t)
}
