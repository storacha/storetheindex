# storoku:ignore

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
    }
    archive = {
      source = "hashicorp/archive"
    }
  }
  backend "s3" {
    bucket = "storacha-terraform-state"
    region = "us-west-2"
    encrypt = true
  }
}

provider "aws" {
  allowed_account_ids = [var.allowed_account_id]
  region = var.region
  default_tags {
    
    tags = {
      "Environment" = terraform.workspace
      "ManagedBy"   = "OpenTofu"
      Owner         = "storacha"
      Team          = "Storacha Engineering"
      Organization  = "Storacha"
      Project       = "${var.app}"
    }
  }
}

# CloudFront is a global service. Certs must be created in us-east-1, where the core ACM infra lives
provider "aws" {
  region = "us-east-1"
  alias = "acm"
}

module "app" {
  source = "github.com/storacha/storoku//app?ref=v0.2.33"
  private_key = var.private_key
  private_key_env_var = "STORETHEINDEX_PRIV_KEY"
  httpport = 3000
  principal_mapping = var.principal_mapping
  did = var.did
  app = var.app
  appState = var.app
  write_to_container = true
  environment = terraform.workspace
  # if there are any env vars you want available only to your container
  # in the vpc as opposed to set in the dockerfile, enter them here
  # NOTE: do not put sensitive data in env-vars. use secrets
  deployment_env_vars = []
  image_tag = var.image_tag
  create_db = false
  # enter secret values your app will use here -- these will be available
  # as env vars in the container at runtime
  secrets = {}
  # enter any sqs queues you want to create here
  queues = []
  caches = []
  topics = []
  tables = [
    {
      name = "valuestore-providers"
      attributes = [
        {
          name = "ProviderID"
          type = "S"
        },
      
        {
          name = "ContextID"
          type = "B"
        },
      ]
      hash_key = "ProviderID"
      range_key ="ContextID"
    },
    {
      name = "valuestore-multihash-map"
      attributes = [
        {
          name = "Multihash"
          type = "B"
        },
      
        {
          name = "ValueKey"
          type = "S"
        },
      ]
      hash_key = "Multihash"
      range_key ="ValueKey"
    },
  ]
  buckets = [
    {
      name = "datastore"
      public = false
    },
    {
      name = "tmp-datastore"
      public = false
    },
  ]
  providers = {
    aws = aws
    aws.acm = aws.acm
  }
  env_files = var.env_files
  domain_base = var.domain_base
}

module "ingest" {
  source = "./ingest"
  app = "${var.app}-ingest"
  environment = terraform.workspace
  vpc = module.app.vpc
  ecs_cluster = module.app.ecs_infra.ecs_cluster
  ecs_log_group = module.app.ecs_infra.aws_cloudwatch_log_group
  find_task_family = module.app.deployment.task_definition.family
  lb_listener = module.app.ecs_infra.lb_listener
  lb_security_group = module.app.ecs_infra.lb_security_group
  httpport = 3001
  write_to_container = true
  image_tag = var.image_tag
  env_files = var.ingest_env_files
  env_files_bucket_id = module.app.env_files.bucket_id
  secrets = module.app.secrets
  buckets = module.app.buckets
  tables = module.app.tables
}
