package engine

type Operation string
type Kind uint8

const (
	SafeRead Kind = iota
	Mutation
	Crypto
	Verification
	AuthAction
	HealthProbe
)
const (
	KVCreate            Operation = "KV_CREATE"
	KVCAS               Operation = "KV_CAS"
	KVReadVersion       Operation = "KV_READ_VERSION"
	KVReadLatest        Operation = "KV_READ_LATEST"
	KVMetadata          Operation = "KV_METADATA"
	KVList              Operation = "KV_LIST"
	KVDelete            Operation = "KV_DELETE_VERSIONS"
	KVUndelete          Operation = "KV_UNDELETE_VERSIONS"
	PKIIssue            Operation = "PKI_ISSUE"
	PKISignCSR          Operation = "PKI_SIGN_CSR"
	PKIRead             Operation = "PKI_READ_CERTIFICATE"
	PKIChain            Operation = "PKI_READ_ISSUER_CHAIN"
	PKIRevoke           Operation = "PKI_REVOKE"
	TransitSign         Operation = "TRANSIT_SIGN"
	TransitSignDigest   Operation = "TRANSIT_SIGN_DIGEST"
	TransitVerify       Operation = "TRANSIT_VERIFY"
	TransitVerifyDigest Operation = "TRANSIT_VERIFY_DIGEST"
	TransitPublicKey    Operation = "TRANSIT_PUBLIC_KEY"
	TransitMetadata     Operation = "TRANSIT_METADATA"
	TransitEncrypt      Operation = "TRANSIT_ENCRYPT"
	TransitDecrypt      Operation = "TRANSIT_DECRYPT"
	TransitRewrap       Operation = "TRANSIT_REWRAP"
	TransitHMAC         Operation = "TRANSIT_HMAC"
	TransitHMACVerify   Operation = "TRANSIT_HMAC_VERIFY"
	AppRoleLogin        Operation = "APPROLE_LOGIN"
	TokenRenew          Operation = "TOKEN_RENEW"
	ClusterHealth       Operation = "CLUSTER_HEALTH"
)

type Definition struct {
	Method string
	Kind   Kind
	Void   bool
}

var operations = map[Operation]Definition{
	KVCreate: {"POST", Mutation, false}, KVCAS: {"POST", Mutation, false}, KVReadVersion: {"GET", SafeRead, false}, KVReadLatest: {"GET", SafeRead, false}, KVMetadata: {"GET", SafeRead, false}, KVList: {"GET", SafeRead, false}, KVDelete: {"POST", Mutation, true}, KVUndelete: {"POST", Mutation, true},
	PKIIssue: {"POST", Mutation, false}, PKISignCSR: {"POST", Mutation, false}, PKIRead: {"GET", SafeRead, false}, PKIChain: {"GET", SafeRead, false}, PKIRevoke: {"POST", Mutation, false},
	TransitSign: {"POST", Crypto, false}, TransitSignDigest: {"POST", Crypto, false}, TransitVerify: {"POST", Verification, false}, TransitVerifyDigest: {"POST", Verification, false}, TransitPublicKey: {"GET", SafeRead, false}, TransitMetadata: {"GET", SafeRead, false}, TransitEncrypt: {"POST", Crypto, false}, TransitDecrypt: {"POST", Verification, false}, TransitRewrap: {"POST", Crypto, false}, TransitHMAC: {"POST", Crypto, false}, TransitHMACVerify: {"POST", Verification, false},
	AppRoleLogin: {"POST", AuthAction, false}, TokenRenew: {"POST", AuthAction, false}, ClusterHealth: {"GET", HealthProbe, false},
}

func Lookup(op Operation) (Definition, bool) { d, ok := operations[op]; return d, ok }
func SideEffect(kind Kind) bool              { return kind == Mutation || kind == Crypto || kind == AuthAction }
