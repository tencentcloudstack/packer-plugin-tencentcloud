data "tencentcloud-image" "test-image" {
  filters = {
    image-type = "PRIVATE_IMAGE"
  }
  most_recent = true
  region      = "ap-guangzhou"
}

locals {
  id   = data.tencentcloud-image.test-image.id
  name = data.tencentcloud-image.test-image.name
}

source "null" "basic-example" {
  communicator = "none"
}

build {
  sources = [
    "source.null.basic-example"
  ]

  provisioner "shell-local" {
    inline = [
      "echo id: ${local.id}",
      "echo name: ${local.name}",
    ]
  }
}