package middleware_test

import (
	"github.com/atticplaygroup/pkv/pkg/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test ParseResourceName", func() {
	It("Should parse valid resource", func() {
		resourceName := "accounts/b8801744-173d-42c8-b634-0a0d4063794d" +
			"/streams/274d80f7-2ebb-4b68-9a13-44ee703aff38" +
			"/values/398963a9-7d38-48bc-9657-1dada3eb7390"
		fields, err := middleware.ParseResourceName(resourceName, []string{
			"accounts", "streams", "values",
		})
		Expect(err).To(BeNil())
		Expect(fields).To(HaveLen(3))
		Expect(fields[0]).To(Equal("b8801744-173d-42c8-b634-0a0d4063794d"))
		Expect(fields[1]).To(Equal("274d80f7-2ebb-4b68-9a13-44ee703aff38"))
		Expect(fields[2]).To(Equal("398963a9-7d38-48bc-9657-1dada3eb7390"))
	})
})
