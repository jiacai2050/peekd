package preview

import "testing"

func TestFormatXMLRemovesBlankLines(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preview>
  
  <name>Peekd</name>
  
  <description>Formatted XML preview fixture.</description>

</preview>`)
	want := `<?xml version="1.0" encoding="UTF-8"?>
<preview>
  <name>Peekd</name>
  <description>Formatted XML preview fixture.</description>
</preview>`

	got, err := formatXML(input)
	if err != nil {
		t.Fatalf("format XML: %v", err)
	}
	if got != want {
		t.Fatalf("formatted XML = %q, want %q", got, want)
	}
}
