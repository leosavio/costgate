package catalog

import (
	"fmt"
	"sort"
)

// Scope of an offer file: one region, or the global file of the service.
const (
	ScopeRegion = "region"
	ScopeGlobal = "global"
)

// Rule selects on-demand rows of one offer file. UsageType is compared after
// the region prefix is removed (see regionPrefixes). Operation is an exact
// match, including the empty string. Match lists extra CSV columns that must
// equal the given value.
type Rule struct {
	Service     string
	Scope       string
	UsageType   string
	UsagePrefix bool
	Operation   string
	Match       map[string]string
}

func (r Rule) String() string {
	op := r.Operation
	if op == "" {
		op = "<empty>"
	}
	ut := r.UsageType
	if r.UsagePrefix {
		ut += "*"
	}
	return fmt.Sprintf("%s/%s usagetype=%s operation=%s", r.Service, r.Scope, ut, op)
}

// regionCodes maps every region of the public Price List (partitions aws and
// aws-us-gov, Local Zones excluded) to its usage type prefix, as the
// AmazonVPC regional offer files use it (checked 2026-11-20: one prefix per
// region). China (aws-cn) has its own Price List endpoint and prices in CNY,
// so it is not here.
var regionCodes = map[string]string{
	"af-south-1":     "AFS1",
	"ap-east-1":      "APE1",
	"ap-east-2":      "APE2",
	"ap-northeast-1": "APN1",
	"ap-northeast-2": "APN2",
	"ap-northeast-3": "APN3",
	"ap-south-1":     "APS3",
	"ap-south-2":     "APS5",
	"ap-southeast-1": "APS1",
	"ap-southeast-2": "APS2",
	"ap-southeast-3": "APS4",
	"ap-southeast-4": "APS6",
	"ap-southeast-5": "APS7",
	"ap-southeast-6": "APS8",
	"ap-southeast-7": "APS9",
	"ca-central-1":   "CAN1",
	"ca-west-1":      "CAN2",
	"eu-central-1":   "EUC1",
	"eu-central-2":   "EUC2",
	"eu-north-1":     "EUN1",
	"eu-south-1":     "EUS1",
	"eu-south-2":     "EUS2",
	"eu-west-1":      "EU",
	"eu-west-2":      "EUW2",
	"eu-west-3":      "EUW3",
	"il-central-1":   "ILC1",
	"me-central-1":   "MEC1",
	"me-south-1":     "MES1",
	"mx-central-1":   "MXC1",
	"sa-east-1":      "SAE1",
	"us-east-1":      "USE1",
	"us-east-2":      "USE2",
	"us-gov-east-1":  "UGE1",
	"us-gov-west-1":  "UGW1",
	"us-west-1":      "USW1",
	"us-west-2":      "USW2",
}

// regionPrefixes lists the usage type prefixes AWS uses for a region. The
// Price List is not consistent (EC2 in us-east-1 has no prefix, VPC uses
// "USE1-", KMS uses "us-east-1-"), so every known form is stripped.
func regionPrefixes(region string) ([]string, bool) {
	code, ok := regionCodes[region]
	if !ok {
		return nil, false
	}
	return []string{code + "-", region + "-"}, true
}

// RegionSupported reports whether refresh knows the usage type prefixes of region.
func RegionSupported(region string) bool {
	_, ok := regionCodes[region]
	return ok
}

// Regions lists the regions refresh supports, sorted.
func Regions() []string {
	out := make([]string, 0, len(regionCodes))
	for r := range regionCodes {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func normalizeUsageType(usageType string, prefixes []string) string {
	for _, p := range prefixes {
		if len(usageType) > len(p) && usageType[:len(p)] == p {
			return usageType[len(p):]
		}
	}
	return usageType
}

var ec2LinuxShared = map[string]string{
	"Tenancy":           "Shared",
	"Operating System":  "Linux",
	"Pre Installed S/W": "NA",
	"CapacityStatus":    "Used",
	"License Model":     "No License required",
}

// Rules is the MVP mapping. Each price key the estimator uses comes from one
// of these rules; refresh fails if a fixed rule matches no row.
var Rules = []Rule{
	// EC2: NAT gateway, Linux on-demand instances (x86 and Graviton), EBS storage.
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "NatGateway-Hours", Operation: "NatGateway"},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "NatGateway-Bytes", Operation: "NatGateway"},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "BoxUsage:", UsagePrefix: true, Operation: "RunInstances", Match: ec2LinuxShared},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeUsage", Operation: ""},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeUsage.gp2", Operation: ""},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeUsage.gp3", Operation: ""},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeUsage.st1", Operation: ""},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeUsage.sc1", Operation: ""},
	// gp3 IOPS above 3000 (IOPS-Mo) and throughput above 125 MiB/s (GiBps-mo).
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeP-IOPS.gp3", Operation: ""},
	{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "EBS:VolumeP-Throughput.gp3", Operation: ""},

	// EKS: control plane hour, extended support hour, Auto Mode management
	// fee per instance type (EKS-Auto:<type>-management-hours).
	{Service: "AmazonEKS", Scope: ScopeRegion, UsageType: "AmazonEKS-Hours:perCluster", Operation: "CreateOperation"},
	{Service: "AmazonEKS", Scope: ScopeRegion, UsageType: "AmazonEKS-Hours:extendedSupport", Operation: "ExtendedSupport"},
	{Service: "AmazonEKS", Scope: ScopeRegion, UsageType: "EKS-Auto:", UsagePrefix: true, Operation: "EKSAutoUsage"},

	// VPC: public IPv4, interface endpoints, Transit Gateway, Site-to-Site VPN.
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "PublicIPv4:InUseAddress", Operation: ""},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "VpcEndpoint-Hours", Operation: "VpcEndpoint"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "VpcEndpoint-Bytes", Operation: "VpcEndpoint"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "TransitGateway-Hours", Operation: "TransitGatewayVPC"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "TransitGateway-Bytes", Operation: "TransitGatewayVPC"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "TransitGateway-Hours", Operation: "TransitGatewayPeering"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "TransitGateway-Hours", Operation: "TransitGatewayVPN"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "TransitGateway-Bytes", Operation: "TransitGatewayVPN"},
	{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "VPN-Usage-Hours:ipsec.1", Operation: "CreateVpnConnection"},

	// Route 53: hosted zones (global offer) and Resolver endpoint ENIs.
	{Service: "AmazonRoute53", Scope: ScopeGlobal, UsageType: "HostedZone", Operation: ""},
	{Service: "AmazonRoute53", Scope: ScopeRegion, UsageType: "ResolverNetworkInterface", Operation: ""},

	// KMS, S3 Standard, CloudTrail, AWS Private CA.
	{Service: "awskms", Scope: ScopeRegion, UsageType: "KMS-Keys", Operation: ""},
	{Service: "AmazonS3", Scope: ScopeRegion, UsageType: "TimedStorage-ByteHrs", Operation: ""},
	{Service: "AWSCloudTrail", Scope: ScopeRegion, UsageType: "PaidEventsRecorded", Operation: ""},
	{Service: "AWSCertificateManager", Scope: ScopeRegion, UsageType: "PaidPrivateCA", Operation: ""},
	{Service: "AWSCertificateManager", Scope: ScopeRegion, UsageType: "ShortLivedCertificatePrivateCA", Operation: ""},
}
