module github.com/terraform-providers/terraform-provider-tfe

replace github.com/hashicorp/go-tfe => ../../hashicorp/go-tfe

require (
	github.com/hashicorp/go-tfe v0.8.2
	github.com/hashicorp/go-version v1.2.0
	github.com/hashicorp/hcl v0.0.0-20180404174102-ef8a98b0bbce
	github.com/hashicorp/terraform-plugin-sdk v1.0.0
	github.com/hashicorp/terraform-svchost v0.0.0-20191011084731-65d371908596
	github.com/ryboe/q v1.0.11
)
