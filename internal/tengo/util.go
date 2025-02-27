package tengo

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// EscapeIdentifier is for use in safely escaping MySQL identifiers (table
// names, column names, etc). It doubles any backticks already present in the
// input string, and then returns the string wrapped in outer backticks.
func EscapeIdentifier(input string) string {
	escaped := strings.Replace(input, "`", "``", -1)
	return fmt.Sprintf("`%s`", escaped)
}

// EscapeValueForCreateTable returns the supplied value (typically obtained from
// querying an information_schema table) escaped in the same manner as SHOW
// CREATE TABLE would display it. Examples include default values, table
// comments, column comments, index comments.
func EscapeValueForCreateTable(input string) string {
	replacements := []struct{ old, new string }{
		{"\\", "\\\\"},
		{"\000", "\\0"},
		{"'", "''"},
		{"\n", "\\n"},
		{"\r", "\\r"},
	}
	for _, operation := range replacements {
		input = strings.Replace(input, operation.old, operation.new, -1)
	}
	return input
}

// SplitHostOptionalPort takes an address string containing a hostname, ipv4
// addr, or ipv6 addr; *optionally* followed by a colon and port number. It
// splits the hostname portion from the port portion and returns them
// separately. If no port was present, 0 will be returned for that portion.
// If hostaddr contains an ipv6 address, the IP address portion must be
// wrapped in brackets on input, and the brackets will still be present on
// output.
func SplitHostOptionalPort(hostaddr string) (string, int, error) {
	if len(hostaddr) == 0 {
		return "", 0, errors.New("Cannot parse blank host address")
	}

	// ipv6 without port, or ipv4 or hostname without port
	if (hostaddr[0] == '[' && hostaddr[len(hostaddr)-1] == ']') || len(strings.Split(hostaddr, ":")) == 1 {
		return hostaddr, 0, nil
	}

	host, portString, err := net.SplitHostPort(hostaddr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, err
	} else if port < 1 {
		return "", 0, fmt.Errorf("invalid port %d supplied", port)
	}

	// ipv6 with port: add the brackets back in -- net.SplitHostPort removes them,
	// but we still need them to form a valid DSN later
	if hostaddr[0] == '[' && host[0] != '[' {
		host = fmt.Sprintf("[%s]", host)
	}

	return host, port, nil
}

var reParseTablespace = regexp.MustCompile(`[)] /\*!50100 TABLESPACE ` + "`((?:[^`]|``)+)`" + ` \*/ ENGINE=`)

// ParseCreateTablespace parses a TABLESPACE clause out of a CREATE TABLE
// statement.
func ParseCreateTablespace(createStmt string) string {
	matches := reParseTablespace.FindStringSubmatch(createStmt)
	if matches != nil {
		return matches[1]
	}
	return ""
}

var reParseCreateAutoInc = regexp.MustCompile(`[)/] ENGINE=\w+ (AUTO_INCREMENT=(\d+) )DEFAULT CHARSET=`)

// ParseCreateAutoInc parses a CREATE TABLE statement, formatted in the same
// manner as SHOW CREATE TABLE, and removes the table-level next-auto-increment
// clause if present. The modified CREATE TABLE will be returned, along with
// the next auto-increment value if one was found.
func ParseCreateAutoInc(createStmt string) (string, uint64) {
	matches := reParseCreateAutoInc.FindStringSubmatch(createStmt)
	if matches == nil {
		return createStmt, 0
	}
	nextAutoInc, _ := strconv.ParseUint(matches[2], 10, 64)
	newStmt := strings.Replace(createStmt, matches[1], "", 1)
	return newStmt, nextAutoInc
}

var reParseCreatePartitioning = regexp.MustCompile(`(?is)(\s*(?:/\*!?\d*)?\s*partition\s+by .*)$`)

// ParseCreatePartitioning parses a CREATE TABLE statement, formatted in the
// same manner as SHOW CREATE TABLE, and splits out the base CREATE clauses from
// the partioning clause.
func ParseCreatePartitioning(createStmt string) (base, partitionClause string) {
	matches := reParseCreatePartitioning.FindStringSubmatch(createStmt)
	if matches == nil {
		return createStmt, ""
	}
	return createStmt[0 : len(createStmt)-len(matches[1])], matches[1]
}

// reformatCreateOptions converts a value obtained from
// information_schema.tables.create_options to the formatting used in SHOW
// CREATE TABLE.
func reformatCreateOptions(input string) string {
	if input == "" {
		return ""
	}
	options := strings.Split(input, " ")
	result := make([]string, 0, len(options))

	for _, kv := range options {
		tokens := strings.SplitN(kv, "=", 2)
		// Option name always all caps in SHOW CREATE TABLE, *except* for backtick-
		// wrapped option names in MariaDB, which preserve the capitalization supplied
		// by the user
		if tokens[0][0] != '`' {
			tokens[0] = strings.ToUpper(tokens[0])
		}
		if len(tokens) == 1 {
			// Partitioned tables have "partitioned" in this field, but partitioning
			// information is contained in a different spot in SHOW CREATE TABLE
			if tokens[0] != "PARTITIONED" {
				result = append(result, tokens[0])
			}
			continue
		}

		// Double quote wrapper changed to single quotes in SHOW CREATE TABLE
		if tokens[1][0] == '"' && tokens[1][len(tokens[1])-1] == '"' {
			tokens[1] = fmt.Sprintf("'%s'", tokens[1][1:len(tokens[1])-1])
		}
		result = append(result, fmt.Sprintf("%s=%s", tokens[0], tokens[1]))
	}
	return strings.Join(result, " ")
}

var normalizeCreateRegexps = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{re: regexp.MustCompile(" /\\*!50606 (STORAGE|COLUMN_FORMAT) (DISK|MEMORY|FIXED|DYNAMIC) \\*/"), replacement: ""},
	{re: regexp.MustCompile(" USING (HASH|BTREE)"), replacement: ""},
	{re: regexp.MustCompile("`\\) KEY_BLOCK_SIZE=\\d+"), replacement: "`)"},
}

// NormalizeCreateOptions adjusts the supplied CREATE TABLE statement to remove
// any no-op table options that are persisted in SHOW CREATE TABLE, but not
// reflected in information_schema and serve no purpose for InnoDB tables.
// This function is not guaranteed to be safe for non-InnoDB tables.
func NormalizeCreateOptions(createStmt string) string {
	for _, entry := range normalizeCreateRegexps {
		createStmt = entry.re.ReplaceAllString(createStmt, entry.replacement)
	}
	return createStmt
}

// StripDisplayWidth examines the supplied column type, and removes its integer
// display width if it is either an int family type, or a YEAR(4) type. No
// change is made if the column type isn't one that has a notion of integer
// display width. Additionally, no change is made to tinyint(1) types, nor
// types with a zerofill modifier, as per handling in MySQL 8.0.19.
func StripDisplayWidth(colType string) (strippedType string, didStrip bool) {
	input := strings.ToLower(colType)
	if !strings.Contains(input, "int(") && input != "year(4)" {
		return colType, false
	} else if input == "tinyint(1)" || strings.HasSuffix(input, "zerofill") {
		return colType, false
	}
	openParen := strings.IndexRune(colType, '(')
	var modifier string
	if strings.HasSuffix(input, " unsigned") {
		modifier = " unsigned"
	}
	return colType[0:openParen] + modifier, true
}

// baseDSN returns a DSN with the database (schema) name and params stripped.
// Currently only supports MySQL, via go-sql-driver/mysql's DSN format.
func baseDSN(dsn string) string {
	tokens := strings.SplitAfter(dsn, "/")
	return strings.Join(tokens[0:len(tokens)-1], "")
}

// paramMap builds a map representing all params in the DSN.
// This does not rely on mysql.ParseDSN because that handles some vars
// separately; i.e. mysql.Config's params field does NOT include all
// params that are passed in!
func paramMap(dsn string) map[string]string {
	parts := strings.Split(dsn, "?")
	if len(parts) == 1 {
		return make(map[string]string)
	}
	params := parts[len(parts)-1]
	values, _ := url.ParseQuery(params)

	// Convert values, which is map[string][]string, to single-valued map[string]string
	// i.e. if a param is present multiple times, we only keep the first value
	result := make(map[string]string, len(values))
	for key := range values {
		result[key] = values.Get(key)
	}
	return result
}

// MergeParamStrings combines two query-string-style formatted DB connection
// parameter strings, with the latter string overriding the former in cases of
// conflicts.
// This is inefficient and should be avoided in hot paths; eventually we will
// move away from DSNs and use Connectors instead, which will remove the need
// for this logic.
func MergeParamStrings(params, overrides string) string {
	v, _ := url.ParseQuery(params)
	v2, _ := url.ParseQuery(overrides)
	for name := range v2 {
		v.Set(name, v2.Get(name))
	}
	return v.Encode()
}

// longestIncreasingSubsequence implements an algorithm useful in computing
// diffs for column order or trigger order.
func longestIncreasingSubsequence(input []int) []int {
	if len(input) < 2 {
		return input
	}
	candidateLists := make([][]int, 1, len(input))
	candidateLists[0] = []int{input[0]}
	for i := 1; i < len(input); i++ {
		comp := input[i]
		if comp < candidateLists[0][0] {
			candidateLists[0][0] = comp
		} else if longestList := candidateLists[len(candidateLists)-1]; comp > longestList[len(longestList)-1] {
			newList := make([]int, len(longestList)+1)
			copy(newList, longestList)
			newList[len(longestList)] = comp
			candidateLists = append(candidateLists, newList)
		} else {
			for j := len(candidateLists) - 2; j >= 0; j-- {
				if thisList, nextList := candidateLists[j], candidateLists[j+1]; comp > thisList[len(thisList)-1] {
					copy(nextList, thisList)
					nextList[len(nextList)-1] = comp
					break
				}
			}
		}
	}
	return candidateLists[len(candidateLists)-1]
}
