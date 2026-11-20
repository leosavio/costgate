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
	rule     string // BillingFixedHourly, BillingFixedMonthly or BillingUsage
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
	keyEKSCluster    = catalog.Key("AmazonEKS", "AmazonEKS-Hours:perCluster", "CreateOperation")
	keyEKSExtended   = catalog.Key("AmazonEKS", "AmazonEKS-Hours:extendedSupport", "ExtendedSupport")
	keySecret        = catalog.Key("AWSSecretsManager", "AWSSecretsManager-Secrets", "")
)

// eksTiers maps control_plane_scaling_config.tier to the code of its
// provisioned tier usage type.
var eksTiers = map[string]string{"tier-xl": "XL", "tier-2xl": "2XL", "tier-4xl": "4XL", "tier-8xl": "8XL"}

// eksTierKey is the hour of a provisioned control plane tier.
func eksTierKey(code string) string {
	return catalog.Key("AmazonEKS", "AmazonEKS-Hours:"+code+"-ProvisionedTier", "ProvisionedControlPlaneUsage")
}

// eksAutoModeKey is the EKS Auto Mode management fee of one instance type.
func eksAutoModeKey(instanceType string) string {
	return catalog.Key("AmazonEKS", "EKS-Auto:"+instanceType+"-management-hours", "EKSAutoUsage")
}

// gp3 performance above the baseline included in the GB-month price.
var (
	keyGP3IOPS        = catalog.Key("AmazonEC2", "EBS:VolumeP-IOPS.gp3", "")
	keyGP3Throughput  = catalog.Key("AmazonEC2", "EBS:VolumeP-Throughput.gp3", "")
	gp3BaseIOPS       = money.Int(3000)
	gp3BaseThroughput = money.Int(125) // MiB/s
	mibPerGiB         = money.Int(1024)
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
	"aws_eks_cluster":                            eksCluster,
	"aws_eks_node_group":                         eksNodeGroup,
	"aws_secretsmanager_secret":                  secret,
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
	"aws_secretsmanager_secret_version": true, "aws_secretsmanager_secret_policy": true,
	"aws_secretsmanager_secret_rotation": true,
	"terraform_data":                     true, "null_resource": true,
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
	return comp{name: name, key: key, qty: q, unit: "h", declared: true, rule: BillingFixedHourly}
}

func gbComp(name, key string, u usage.Resolved) comp {
	return comp{name: name, key: key, qty: u.GB.V, unit: "GB", premise: "gb_per_month", declared: u.GB.Declared, rule: BillingUsage}
}

func monthly(name, key, unit, note string) estimator {
	return func(attrs, usage.Resolved) side {
		return side{comps: []comp{{name: name, key: key, qty: money.Int(1), unit: unit, declared: true, note: note, rule: BillingFixedMonthly}}}
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

// volumeComps prices one EBS volume: the GB-month line, and for gp3 the
// IOPS above 3000 and the throughput above 125 MiB/s, which AWS bills on
// their own usage types. The Price List bills throughput per GiBps-month,
// so the MiB/s above the baseline are divided by 1024.
func volumeComps(name, volType string, size *big.Rat, iops, throughput *big.Rat) ([]comp, string) {
	ut, ok := ebsUsageTypes[volType]
	if !ok {
		return nil, fmt.Sprintf("EBS volume type %q is not modeled (provisioned IOPS)", volType)
	}
	out := []comp{{name: name + " (" + volType + ")", key: catalog.Key("AmazonEC2", ut, ""), qty: size, unit: "GB-month", declared: true, rule: BillingFixedMonthly}}
	if volType != "gp3" {
		return out, ""
	}
	if iops != nil && iops.Cmp(gp3BaseIOPS) > 0 {
		out = append(out, comp{name: name + " IOPS above 3000 (gp3)", key: keyGP3IOPS, qty: money.Sub(iops, gp3BaseIOPS),
			unit: "IOPS-month", declared: true, rule: BillingFixedMonthly, note: money.Trim(iops) + " IOPS provisioned, 3000 included"})
	}
	if throughput != nil && throughput.Cmp(gp3BaseThroughput) > 0 {
		extra := money.Sub(throughput, gp3BaseThroughput)
		out = append(out, comp{name: name + " throughput above 125 MiB/s (gp3)", key: keyGP3Throughput,
			qty: new(big.Rat).Quo(extra, mibPerGiB), unit: "GiBps-month", declared: true, rule: BillingFixedMonthly,
			note: money.Trim(throughput) + " MiB/s provisioned, 125 included; " + money.Trim(extra) + " MiB/s ÷ 1024"})
	}
	return out, ""
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
			// The Spot price is set by the market, not by the price list.
			c.rule = BillingUsage
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
		rc, why := volumeComps("root volume", volType, size, iops, tp)
		if why != "" {
			return unpriced("root_block_device: %s", why)
		}
		rc[0].note = note
		s.comps = append(s.comps, rc...)

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
			dc, why := volumeComps(fmt.Sprintf("ebs_block_device[%d]", i), vt, sz, io, th)
			if why != "" {
				return unpriced("ebs_block_device[%d]: %s", i, why)
			}
			s.comps = append(s.comps, dc...)
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
	c, why := volumeComps("EBS volume", volType, size, iops, tp)
	if why != "" {
		return unpriced("%s", why)
	}
	c[0].note = note
	return side{comps: c}
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
		premise: "gb_per_month", declared: u.GB.Declared, note: "first 50 TB tier; requests not priced", rule: BillingUsage}
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
		premise: "events_per_month", declared: u.Events.Declared, rule: BillingUsage,
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
		return side{comps: []comp{{name: "Private CA (short-lived mode)", key: keyPCAShortLived, qty: money.Int(1), unit: "CA-month", declared: true, note: "certificates not priced", rule: BillingFixedMonthly}}}
	}
	return side{comps: []comp{{name: "Private CA (general-purpose mode)", key: keyPCAGeneral, qty: money.Int(1), unit: "CA-month", declared: true, note: "certificates not priced", rule: BillingFixedMonthly}}}
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

// eksCluster prices the control plane hour (standard support). Extended
// support and the EKS Auto Mode management fee depend on facts the plan does
// not carry (the Kubernetes version calendar, the nodes Auto Mode launches),
// so they come only from declared premises.
func eksCluster(a attrs, u usage.Resolved) side {
	if l, ok := a.list("outpost_config"); ok && len(l) > 0 {
		return unpriced("local cluster on AWS Outposts (outpost_config) is not modeled")
	}
	// The provisioned control plane tier is billed by the hour on top of the
	// standard or extended support hour. A tier changed outside Terraform is
	// not in the plan.
	var tierComp *comp
	tierNote := "standard tier: the plan has no control_plane_scaling_config"
	if sc, ok := a.first("control_plane_scaling_config"); ok {
		if sc.isUnknown("tier") {
			return unpriced("control_plane_scaling_config.tier unknown at plan time")
		}
		tier, _ := sc.str("tier")
		tierNote = ""
		if tier != "" && tier != "standard" {
			code, ok := eksTiers[tier]
			if !ok {
				return unpriced("provisioned control plane tier %q is not modeled", tier)
			}
			c := hourly("provisioned control plane, "+tier, eksTierKey(code), u, nil)
			c.note = "billed on top of the control plane hour"
			tierComp = &c
		}
	}
	cp := hourly("cluster control plane, standard support", keyEKSCluster, u, nil)
	var notes []string
	if tierNote != "" {
		notes = append(notes, tierNote)
	}
	var s side
	if u.ExtendedSupportHours.Declared {
		cp.note = strings.Join(notes, "; ")
		s.comps = append(s.comps, cp, comp{name: "extended support", key: keyEKSExtended, qty: u.ExtendedSupportHours.V, unit: "h",
			premise: "extended_support_hours", declared: true, rule: BillingUsage, note: "billed on top of the control plane hour"})
	} else {
		cp.note = strings.Join(append(notes, "extended support not included; declare extended_support_hours for a version in extended support"), "; ")
		s.comps = append(s.comps, cp)
	}
	if tierComp != nil {
		s.comps = append(s.comps, *tierComp)
	}
	cc, ok := a.first("compute_config")
	if on, _ := cc.boolean("enabled"); !ok || !on {
		return s
	}
	am := u.AutoModeInstanceHours
	if !am.Declared {
		s.comps = append(s.comps, comp{name: "EKS Auto Mode management fee", qty: money.Zero(), unit: "h",
			premise: "auto_mode_instance_hours", rule: BillingUsage,
			note: "per instance type and hour of the nodes Auto Mode launches, which the plan does not carry"})
		return s
	}
	for _, it := range am.Keys() {
		s.comps = append(s.comps, comp{name: "EKS Auto Mode management fee, " + it, key: eksAutoModeKey(it), qty: am.V[it], unit: "h",
			premise: "auto_mode_instance_hours", declared: true, rule: BillingUsage, note: "EC2 cost of the nodes not included"})
	}
	return s
}

// eksNodeGroup prices a managed node group as desired_size Linux instances
// of its first instance type, plus their disks. An autoscaler moves
// desired_size after apply; the estimate is the size the plan sets.
func eksNodeGroup(a attrs, u usage.Resolved) side {
	if at, _ := a.str("ami_type"); strings.HasPrefix(at, "WINDOWS_") || at == "CUSTOM" {
		return unpriced("ami_type %q is not modeled (only Amazon Linux and Bottlerocket)", at)
	}
	sc, _ := a.first("scaling_config")
	desired, ok := sc.number("desired_size")
	if !ok {
		return unpriced("scaling_config.desired_size unknown at plan time")
	}
	_, hasLT := a.first("launch_template")
	types, _ := a.list("instance_types")
	var notes []string
	var it string
	switch {
	case len(types) > 0:
		it, _ = types[0].(string)
		if len(types) > 1 {
			notes = append(notes, fmt.Sprintf("first of %d instance_types", len(types)))
		}
	case a.isUnknown("instance_types"):
		return unpriced("instance_types unknown at plan time")
	case hasLT:
		return unpriced("instance type set by the launch template, which the node group does not carry in the plan")
	default:
		it = "t3.medium"
		notes = append(notes, "instance_types not set: EKS default t3.medium")
	}
	if it == "" {
		return unpriced("instance_types unknown at plan time")
	}
	notes = append(notes, money.Trim(desired)+" nodes from scaling_config.desired_size, which an autoscaler changes after apply")
	c := hourly("compute on-demand, Linux "+it, instanceKey(it), u, desired)
	if ct, _ := a.str("capacity_type"); ct == "SPOT" {
		c.name = "compute Spot, Linux " + it
		c.factor = money.Sub(money.Int(1), u.SpotDiscount.V)
		c.premise = "spot_discount"
		c.declared = true
		c.rule = BillingUsage
		notes = append(notes, "on-demand × (1 − spot_discount "+money.Trim(u.SpotDiscount.V)+")")
	}
	c.note = strings.Join(notes, "; ")
	s := side{comps: []comp{c}}

	// Without a launch template, EKS creates one with disk_size GiB (20 by
	// default for Linux) of gp2; with one, the disk is in the launch template.
	size, sizeOK := a.number("disk_size")
	note := "gp2, the volume type of the launch template EKS creates"
	switch {
	case hasLT:
		size = u.RootVolumeGB.V
		note = "disk set by the launch template; root_volume_gb premise " + money.Trim(size) + " GB (" + u.RootVolumeGB.From + "), gp2 assumed"
	case !sizeOK:
		size = money.Int(20)
		note = "disk_size not set: EKS default 20 GB; " + note
	}
	dc, why := volumeComps("node disks", "gp2", money.Mul(size, desired), nil, nil)
	if why != "" {
		return unpriced("node disk: %s", why)
	}
	dc[0].note = money.Trim(size) + " GB × " + money.Trim(desired) + " nodes; " + note
	s.comps = append(s.comps, dc...)
	return s
}

// secret prices a Secrets Manager secret and its replicas per month. Each
// replica is billed in its own region; it is priced here at the price of the
// primary region.
func secret(a attrs, u usage.Resolved) side {
	s := side{comps: []comp{{name: "secret", key: keySecret, qty: money.Int(1), unit: "secret-month", declared: true,
		rule: BillingFixedMonthly, note: "API requests not priced"}}}
	if reps, _ := a.list("replica"); len(reps) > 0 {
		s.comps = append(s.comps, comp{name: "replica secrets", key: keySecret, qty: money.Int(int64(len(reps))), unit: "secret-month",
			declared: true, rule: BillingFixedMonthly, note: "at the price of the primary region; each replica is billed in its region"})
	}
	return s
}

func accessAnalyzer(a attrs, u usage.Resolved) side {
	t, _ := a.str("type")
	switch t {
	case "", "ACCOUNT", "ORGANIZATION":
		return side{}
	}
	return unpriced("analyzer type %q (unused or internal access) is not modeled", t)
}
