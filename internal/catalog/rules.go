package catalog

import "fmt"

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

// regionPrefixes lists the usage type prefixes AWS uses for a region. The
// Price List is not consistent (EC2 in us-east-1 has no prefix, VPC uses
// "USE1-", KMS uses "us-east-1-"), so every known form is stripped.
var regionPrefixes = map[string][]string{
	"us-east-1": {"USE1-", "us-east-1-"},
	"us-east-2": {"USE2-", "us-east-2-"},
	"us-west-1": {"USW1-", "us-west-1-"},
	"us-west-2": {"USW2-", "us-west-2-"},
}

// RegionSupported reports whether refresh knows the usage type prefixes of region.
func RegionSupported(region string) bool {
	_, ok := regionPrefixes[region]
	return ok
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
