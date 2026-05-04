package cli

import "fmt"

// RunVersion은 main에서 ldflags로 주입한 version을 출력한다.
func RunVersion(version string) {
	fmt.Printf("quantbot %s\n", version)
}
