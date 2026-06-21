package mysqlcommon

import "testing"

func TestForkValuesAreDistinct(t *testing.T) {
	forks := []Fork{ForkUnknown, ForkMySQL, ForkMariaDB}
	seen := make(map[Fork]bool, len(forks))
	for _, f := range forks {
		if seen[f] {
			t.Fatalf("duplicate Fork value: %d", f)
		}
		seen[f] = true
	}
}
