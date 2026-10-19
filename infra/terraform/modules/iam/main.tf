variable "prefijo" { type = string }
variable "bucket_datos_arn" { type = string }
variable "ssm_prefijo_arn" { type = string }

data "aws_iam_policy_document" "asumir" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "${var.prefijo}-lambda"
  assume_role_policy = data.aws_iam_policy_document.asumir.json
}

# Permisos mínimos y explícitos. La función hace tres cosas: leer y escribir objetos en SU
# bucket, leer sus parámetros de SSM, y escribir sus logs.
data "aws_iam_policy_document" "permisos" {
  statement {
    sid       = "ObjetosDelBucket"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["${var.bucket_datos_arn}/*"]
  }
  statement {
    sid       = "ListarElBucket"
    actions   = ["s3:ListBucket"]
    resources = [var.bucket_datos_arn]
  }
  statement {
    sid       = "LeerSecretos"
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = [var.ssm_prefijo_arn]
  }
  statement {
    sid       = "Logs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents", "logs:CreateLogGroup"]
    resources = ["arn:aws:logs:*:*:*"]
  }
}

resource "aws_iam_role_policy" "lambda" {
  name   = "${var.prefijo}-permisos"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.permisos.json
}

output "rol_lambda_arn" { value = aws_iam_role.lambda.arn }
