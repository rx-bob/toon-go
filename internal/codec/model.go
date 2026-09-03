package codec

// normalizedValue represents a value that has been normalized according to the
// TOON data model and is ready for emission by the encoder.
type normalizedValue interface{}

// fieldNode is one entry in a tabular header field tree. A node with no
// children is a leaf; children describe a nested object group.
type fieldNode struct {
	name     string
	children []fieldNode
}

func (n fieldNode) leaves() []string {
	if len(n.children) == 0 {
		return []string{n.name}
	}
	var result []string
	for _, child := range n.children {
		result = append(result, child.leaves()...)
	}
	return result
}

func flattenFields(fields []fieldNode) []string {
	var result []string
	for _, field := range fields {
		result = append(result, field.leaves()...)
	}
	return result
}

// numberValue captures a numeric literal that should be rendered verbatim.
type numberValue struct {
	literal string
}

// maxSafeInteger mirrors JavaScript's Number.MAX_SAFE_INTEGER, the threshold at
// which IEEE 754 double precision can no longer represent integers exactly.
const maxSafeInteger = 9007199254740991
