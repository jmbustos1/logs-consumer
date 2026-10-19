# Infraestructura de logs-consumer. Un solo `apply` levanta las dos mitades: la API en Lambda
# y el panel estático en S3+CloudFront.
#
# Por qué NO se parece a la de ewood: ewood usa EC2 + RDS porque tiene tráfico constante y un
# modelo relacional. Acá el tráfico son ~150 POST al mes y las consultas de cuando abrís el
# panel. Sin base de datos y sin servidor encendido, la infra son cuatro cosas: una imagen, una
# función, dos buckets y una distribución.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = ">= 5.0" }
  }
}

provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile

  # La cuenta tiene varios proyectos. Sin esto, en la consola de costos todo aparece junto.
  # Son las mismas etiquetas que emiten los pipelines en sus sobres, así que se puede cruzar
  # el gasto con las corridas sin trabajo extra.
  default_tags {
    tags = {
      Proyecto = var.proyecto
      Pipeline = var.pipeline
      Gestion  = "terraform"
    }
  }
}

# CloudFront exige que sus certificados vivan en us-east-1, pase lo que pase con var.aws_region.
provider "aws" {
  alias   = "us_east_1"
  region  = "us-east-1"
  profile = var.aws_profile
}

data "aws_caller_identity" "actual" {}

locals {
  prefijo = "${var.proyecto}-${var.pipeline}"

  cuenta       = data.aws_caller_identity.actual.account_id
  bucket_datos = var.bucket_datos != "" ? var.bucket_datos : "${var.proyecto}-datos-${local.cuenta}"
  bucket_web   = var.bucket_web != "" ? var.bucket_web : "${var.proyecto}-panel-${local.cuenta}"
}

module "ecr" {
  source  = "./modules/ecr"
  nombre  = local.prefijo
  retener = var.imagenes_a_retener
}

# Los datos: sobres crudos, resúmenes y el estado de cada pipeline. Versionado activado porque
# el sobre crudo es la fuente de verdad del sistema; si se pisa por error, se recupera.
module "s3_datos" {
  source         = "./modules/s3-datos"
  bucket         = var.bucket_datos
  dias_retencion = var.dias_retencion_sobres
}

module "s3_web" {
  source = "./modules/s3-web"
  bucket = local.bucket_web
}

module "ssm" {
  source  = "./modules/ssm"
  prefijo = local.prefijo
  # Los valores NO se ponen acá. Terraform los crea vacíos y se cargan con la CLI, para que no
  # terminen en el tfstate, que es texto plano.
  parametros = ["token_emisor", "token_lector", "anthropic_api_key"]
}

module "iam" {
  source           = "./modules/iam"
  prefijo          = local.prefijo
  bucket_datos_arn = module.s3_datos.arn
  ssm_prefijo_arn  = module.ssm.prefijo_arn
}

module "lambda" {
  source              = "./modules/lambda"
  prefijo             = local.prefijo
  imagen              = "${module.ecr.url}:${var.tag_imagen}"
  rol_arn             = module.iam.rol_lambda_arn
  bucket_datos        = module.s3_datos.nombre
  memoria_mb          = var.memoria_mb
  timeout_s           = var.timeout_s
  dias_retencion_logs = var.dias_retencion_logs
  origenes_cors       = var.dominio_panel != "" ? "https://${var.dominio_panel}" : "*"
  diagnostico         = var.diagnostico
  modelo              = var.modelo
  ssm_prefijo         = module.ssm.prefijo
}

module "cloudfront" {
  source             = "./modules/cloudfront"
  prefijo            = local.prefijo
  bucket_web_id      = module.s3_web.id
  bucket_web_dominio = module.s3_web.dominio_regional
  lambda_url         = module.lambda.function_url
  dominio            = var.dominio_panel
  certificado_arn    = var.certificado_arn
}

# Permite que solo esta distribución lea el bucket del panel.
resource "aws_s3_bucket_policy" "web" {
  bucket = module.s3_web.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "cloudfront.amazonaws.com" }
      Action    = "s3:GetObject"
      Resource  = "${module.s3_web.arn}/*"
      Condition = {
        StringEquals = { "AWS:SourceArn" = module.cloudfront.arn }
      }
    }]
  })
}

module "budget" {
  source     = "./modules/budget"
  nombre     = local.prefijo
  pipeline   = var.pipeline
  limite_usd = var.presupuesto_usd
  email      = var.email_alertas
}
