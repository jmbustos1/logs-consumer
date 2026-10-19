variable "prefijo" { type = string }
variable "imagen" { type = string }
variable "rol_arn" { type = string }
variable "bucket_datos" { type = string }
variable "memoria_mb" { type = number }
variable "timeout_s" { type = number }
variable "dias_retencion_logs" { type = number }
variable "origenes_cors" { type = string }
variable "diagnostico" { type = string }
variable "modelo" { type = string }
variable "ssm_prefijo" { type = string }

resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/lambda/${var.prefijo}"
  retention_in_days = var.dias_retencion_logs
}

# La API entera en una función. El binario detecta que corre en Lambda y envuelve el MISMO
# http.Handler que usa en local: no hay una segunda versión de las rutas.
resource "aws_lambda_function" "this" {
  function_name = var.prefijo
  role          = var.rol_arn
  package_type  = "Image"
  image_uri     = var.imagen
  memory_size   = var.memoria_mb
  timeout       = var.timeout_s

  environment {
    variables = {
      LC_ENTORNO     = "lambda"
      LC_ALMACEN     = "s3"
      LC_BUCKET      = var.bucket_datos
      LC_CORS        = var.origenes_cors
      LC_DIAGNOSTICO = var.diagnostico
      LC_MODELO      = var.modelo
      LC_SSM_PREFIJO = var.ssm_prefijo
    }
  }

  lifecycle {
    # El código se actualiza con `aws lambda update-function-code`, no con terraform apply:
    # así desplegar una versión nueva no exige tener el estado de terraform a mano.
    ignore_changes = [image_uri]
  }

  depends_on = [aws_cloudwatch_log_group.this]
}

# Function URL en vez de API Gateway. API Gateway aportaría claves de API, planes de uso y
# transformaciones que acá no se usan, y cobra por millón de peticiones además de la Lambda.
# La autenticación la hace la aplicación con sus dos tokens.
resource "aws_lambda_function_url" "this" {
  function_name      = aws_lambda_function.this.function_name
  authorization_type = "NONE"
}

output "nombre" { value = aws_lambda_function.this.function_name }
output "function_url" { value = aws_lambda_function_url.this.function_url }
output "arn" { value = aws_lambda_function.this.arn }
