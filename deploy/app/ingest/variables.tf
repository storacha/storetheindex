variable "app" {
  description = "The name of the application"
  type        = string
}

variable "environment" {
  description = "The environment the deployment will belong to"
  type        = string
}

variable "find_task" {
  description = "The task family for the find task, so that we can fetch its configuration"
  type        = object({
    family = string
    revision = string
  })
}

variable "httpport" {
  type = number
  default = 3001
}

variable "write_to_container" {
  description = "whether applications can write to the container file system"
  type = bool
  default = false
}

variable "deployment_config" {
  type = object({
    cpu = number
    memory = number
    httpport = number
    readonly = bool
  })
  default = null
}

variable "vpc" {
  description = "The VPC to deploy the db in"
  type        = object({
    id                 = string
    cidr_block         = string
    subnet_ids = object({
      public      = list(string)
      private     = list(string)
      db          = list(string)
      elasticache = list(string)
    })
  })
}

variable "lb_listener" {
  type = object({
    arn = string
  })
}

variable "lb_security_group" {
  type = object({
    id = string
  })
}

variable "ecs_cluster" {
  type = object({
    name = string
    id = string
  })
}

variable "ecs_log_group" {
  type = object({
    name = string
    arn = string
  })
}

variable "image_tag" {
  type = string
}

variable "secrets" {
  type = list(object({
    name      = string
    valueFrom = string
  }))
  default = []
}

variable "env_vars" {
  type = list(object({
    name      = string
    value     = string
  }))
  default = []
}

variable "env_files_bucket_id" {
  description = "The ID of the S3 bucket where the environment files are stored"
  type = string
}

variable "env_files" {
  description = "list of environment variable files to upload and use in the container definition (NO SENSITIVE DATA)"
  type = list(string)
  default = []
}

variable "healthcheck" {
  type = bool
  default = false
}

variable "tables" {
  type = map( object({
    id = string
    arn = string
  }))
}

variable "buckets" {
  type = map(object({
    arn = string
    bucket = string
  }))
}
