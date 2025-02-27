package tengo

import (
	"strings"
	"sync"
)

// The functions in this file currently only manage reserved words (a subset of
// keywords). In the future, it may be expanded to include additional functions
// which operate on all keywords, which may be useful in improving the parser,
// as well as for solving issues like #175 and #199.

// This constant is used for determining map capacity for reserved word maps.
// This is padded slightly; currently MySQL 8 has 262 keywords, vs 249 in recent
// MariaDB releases.
const countReservedWordsPerFlavor = 265

var (
	keywordMutex          sync.Mutex
	reservedWordsByFlavor map[Flavor]map[string]bool // lazily created per flavor
)

// ReservedWordMap returns a map which can be used for looking up whether a
// given word is a reserved word in the supplied flavor. Keys in the map are all
// lowercase. If called repeatedly on the same flavor, a reference to the same
// underlying map will be returned each time. The caller should not modify this
// map.
// The returned map is only designed to be accurate in common situations, and
// does not necessarily account for changes in specific point releases
// (especially pre-GA ones), special sql_mode values like MariaDB's ORACLE
// mode support, or flavors that this package does not support.
func ReservedWordMap(flavor Flavor) map[string]bool {
	if reservedWordsByFlavor != nil {
		if rwm := reservedWordsByFlavor[flavor]; rwm != nil {
			return rwm
		}
	}

	keywordMutex.Lock()
	defer keywordMutex.Unlock()

	if reservedWordsByFlavor == nil {
		reservedWordsByFlavor = make(map[Flavor]map[string]bool)
	}
	rwm := make(map[string]bool, countReservedWordsPerFlavor)

	// Add all keywords that are present in both MySQL 5.5 and MariaDB 10.1, which
	// are the oldest flavors that this package supports.
	for _, word := range commonReservedWords {
		rwm[word] = true
	}

	// Now add in flavor-specific keywords
	for word, flavors := range reservedWordsAddedInFlavor {
		for _, flavorAddedIn := range flavors {
			if flavor.Min(flavorAddedIn) {
				rwm[word] = true
				break
			}
		}
	}

	reservedWordsByFlavor[flavor] = rwm
	return rwm
}

// VendorReservedWordMap returns a map containing all reserved words in any
// version of the supplied vendor.
// For additional documentation on the returned map, see ReservedWordMap.
func VendorReservedWordMap(vendor Vendor) map[string]bool {
	flavor := Flavor{Vendor: vendor, Version: Version{65535, 65535, 65535}}
	return ReservedWordMap(flavor)
}

// IsReservedWord returns true if word is a reserved word in flavor, or false
// otherwise. This result is only designed to be accurate in common situations,
// and does not necessarily account for changes in specific point releases
// (especially pre-GA ones), special sql_mode values like MariaDB's ORACLE
// mode support, or flavors that this package does not support.
func IsReservedWord(word string, flavor Flavor) bool {
	reservedWordMap := ReservedWordMap(flavor)
	return reservedWordMap[strings.ToLower(word)]
}

// IsVendorReservedWord returns true if word is a reserved word in ANY version
// of vendor, or false otherwise.
func IsVendorReservedWord(word string, vendor Vendor) bool {
	reservedWordMap := VendorReservedWordMap(vendor)
	return reservedWordMap[strings.ToLower(word)]
}

// Below this point are unexported variables containing keyword lists. If adding
// new keywords to these variables, be sure to only use lowercase!

// These reserved words are present in both MySQL 5.5 and MariaDB 10.1, which
// are the oldest flavors this package supports. This list should not ever
// change, unless it is found to contain mistakes.
var commonReservedWords = []string{
	"accessible",
	"add",
	"all",
	"alter",
	"analyze",
	"and",
	"as",
	"asc",
	"asensitive",
	"before",
	"between",
	"bigint",
	"binary",
	"blob",
	"both",
	"by",
	"call",
	"cascade",
	"case",
	"change",
	"char",
	"character",
	"check",
	"collate",
	"column",
	"condition",
	"constraint",
	"continue",
	"convert",
	"create",
	"cross",
	"current_date",
	"current_time",
	"current_timestamp",
	"current_user",
	"cursor",
	"database",
	"databases",
	"day_hour",
	"day_microsecond",
	"day_minute",
	"day_second",
	"dec",
	"decimal",
	"declare",
	"default",
	"delayed",
	"delete",
	"desc",
	"describe",
	"deterministic",
	"distinct",
	"distinctrow",
	"div",
	"double",
	"drop",
	"dual",
	"each",
	"else",
	"elseif",
	"enclosed",
	"escaped",
	"exists",
	"exit",
	"explain",
	"false",
	"fetch",
	"float",
	"float4",
	"float8",
	"for",
	"force",
	"foreign",
	"from",
	"fulltext",
	"grant",
	"group",
	"having",
	"high_priority",
	"hour_microsecond",
	"hour_minute",
	"hour_second",
	"if",
	"ignore",
	"in",
	"index",
	"infile",
	"inner",
	"inout",
	"insensitive",
	"insert",
	"int",
	"int1",
	"int2",
	"int3",
	"int4",
	"int8",
	"integer",
	"interval",
	"into",
	"is",
	"iterate",
	"join",
	"key",
	"keys",
	"kill",
	"leading",
	"leave",
	"left",
	"like",
	"limit",
	"linear",
	"lines",
	"load",
	"localtime",
	"localtimestamp",
	"lock",
	"long",
	"longblob",
	"longtext",
	"loop",
	"low_priority",
	"master_ssl_verify_server_cert",
	"match",
	"maxvalue",
	"mediumblob",
	"mediumint",
	"mediumtext",
	"middleint",
	"minute_microsecond",
	"minute_second",
	"mod",
	"modifies",
	"natural",
	"not",
	"no_write_to_binlog",
	"null",
	"numeric",
	"on",
	"optimize",
	"option",
	"optionally",
	"or",
	"order",
	"out",
	"outer",
	"outfile",
	"precision",
	"primary",
	"procedure",
	"purge",
	"range",
	"read",
	"reads",
	"read_write",
	"real",
	"references",
	"regexp",
	"release",
	"rename",
	"repeat",
	"replace",
	"require",
	"resignal",
	"restrict",
	"return",
	"revoke",
	"right",
	"rlike",
	"schema",
	"schemas",
	"second_microsecond",
	"select",
	"sensitive",
	"separator",
	"set",
	"show",
	"signal",
	"smallint",
	"spatial",
	"specific",
	"sql",
	"sqlexception",
	"sqlstate",
	"sqlwarning",
	"sql_big_result",
	"sql_calc_found_rows",
	"sql_small_result",
	"ssl",
	"starting",
	"straight_join",
	"table",
	"terminated",
	"then",
	"tinyblob",
	"tinyint",
	"tinytext",
	"to",
	"trailing",
	"trigger",
	"true",
	"undo",
	"union",
	"unique",
	"unlock",
	"unsigned",
	"update",
	"usage",
	"use",
	"using",
	"utc_date",
	"utc_time",
	"utc_timestamp",
	"values",
	"varbinary",
	"varchar",
	"varcharacter",
	"varying",
	"when",
	"where",
	"while",
	"with",
	"write",
	"xor",
	"year_month",
	"zerofill",
}

// Mapping of lowercased reserved words to the flavor(s) that added them. A
// few notes on keeping this list manageable:
//   - We currently do not track point (aka dot or patch) releases here. It's
//     possible this policy may change to better handle MySQL 8, but so far the
//     only edge case in the past few years is "intersect" (reserved in 8.0.31+).
//   - Some of the entries associated with FlavorMariaDB101 were actually
//     introduced prior to that, but this package does not support pre-10.1,
//     so 10.1 is used as a placeholder for simplicity's sake. A few other entries
//     are inconsistently documented by the MariaDB manual, so 10.1 is used as a
//     guess for: "delete_domain_id", "page_checksum", "parse_vcol_expr", and
//     "position".
//   - We don't yet track anything specific to a Variant (e.g. Percona Server).
//   - Some situational cases are omitted, for example "window" is a MariaDB
//     reserved word only in the context of table name *aliases*, which largely
//     means it isn't relevant to this package at this time.
var reservedWordsAddedInFlavor = map[string][]Flavor{
	"get":             {FlavorMySQL56},
	"io_after_gtids":  {FlavorMySQL56},
	"io_before_gtids": {FlavorMySQL56},
	"master_bind":     {FlavorMySQL56},
	"partition":       {FlavorMySQL56, FlavorMariaDB101},

	"generated":       {FlavorMySQL57},
	"optimizer_costs": {FlavorMySQL57},
	"stored":          {FlavorMySQL57},
	"virtual":         {FlavorMySQL57},

	"cube":         {FlavorMySQL80},
	"cume_dist":    {FlavorMySQL80},
	"dense_rank":   {FlavorMySQL80},
	"empty":        {FlavorMySQL80},
	"except":       {FlavorMySQL80, FlavorMariaDB103},
	"first_value":  {FlavorMySQL80},
	"function":     {FlavorMySQL80},
	"grouping":     {FlavorMySQL80},
	"groups":       {FlavorMySQL80},
	"intersect":    {FlavorMySQL80, FlavorMariaDB103},
	"json_table":   {FlavorMySQL80},
	"lag":          {FlavorMySQL80},
	"last_value":   {FlavorMySQL80},
	"lateral":      {FlavorMySQL80},
	"lead":         {FlavorMySQL80},
	"nth_value":    {FlavorMySQL80},
	"ntile":        {FlavorMySQL80},
	"of":           {FlavorMySQL80},
	"over":         {FlavorMySQL80, FlavorMariaDB102},
	"percent_rank": {FlavorMySQL80},
	"rank":         {FlavorMySQL80},
	"recursive":    {FlavorMySQL80, FlavorMariaDB102},
	"row":          {FlavorMySQL80},
	"rows":         {FlavorMySQL80, FlavorMariaDB102},
	"row_number":   {FlavorMySQL80},
	"system":       {FlavorMySQL80},
	"window":       {FlavorMySQL80}, // see comment above re: MariaDB

	"current_role":            {FlavorMariaDB101},
	"delete_domain_id":        {FlavorMariaDB101}, // actual version unclear from docs, see comment above
	"do_domain_ids":           {FlavorMariaDB101},
	"general":                 {FlavorMariaDB101},
	"ignore_domain_ids":       {FlavorMariaDB101},
	"ignore_server_ids":       {FlavorMariaDB101},
	"master_heartbeat_period": {FlavorMariaDB101},
	"page_checksum":           {FlavorMariaDB101}, // actual version unclear from docs, see comment above
	"parse_vcol_expr":         {FlavorMariaDB101}, // actual version unclear from docs, see comment above
	"position":                {FlavorMariaDB101}, // actual version unclear from docs, see comment above
	"ref_system_id":           {FlavorMariaDB101},
	"returning":               {FlavorMariaDB101},
	"slow":                    {FlavorMariaDB101},
	"stats_auto_recalc":       {FlavorMariaDB101},
	"stats_persistent":        {FlavorMariaDB101},
	"stats_sample_pages":      {FlavorMariaDB101},

	"offset": {FlavorMariaDB106},
}
