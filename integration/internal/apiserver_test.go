package internal_test

import (
	. "github.com/kubernetes-sig-testing/frameworks/integration/internal"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Apiserver", func() {
	It("defaults Args if they are empty", func() {
		initialArgs := []string{}
		defaultedArgs := DoAPIServerArgDefaulting(initialArgs)
		Expect(defaultedArgs).To(BeEquivalentTo(APIServerDefaultArgs))
	})

	It("keeps Args as is if they are not empty", func() {
		initialArgs := []string{"--one", "--two=2"}
		defaultedArgs := DoAPIServerArgDefaulting(initialArgs)
		Expect(defaultedArgs).To(BeEquivalentTo([]string{
			"--one", "--two=2",
		}))
	})
})
