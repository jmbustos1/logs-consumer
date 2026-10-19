variable "bucket" { type = string }
variable "dias_retencion" { type = number }

resource "aws_s3_bucket" "this" {
  bucket = var.bucket
}

# El sobre crudo es la fuente de verdad del sistema: todo lo demás —resúmenes, estado de cada
# pipeline— se reconstruye a partir de él. Versionado para que un Put equivocado no lo pierda.
resource "aws_s3_bucket_versioning" "this" {
  bucket = aws_s3_bucket.this.id
  versioning_configuration { status = "Enabled" }
}

resource "aws_s3_bucket_public_access_block" "this" {
  bucket                  = aws_s3_bucket.this.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "this" {
  bucket = aws_s3_bucket.this.id
  rule {
    apply_server_side_encryption_by_default { sse_algorithm = "AES256" }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "this" {
  bucket = aws_s3_bucket.this.id

  # Las versiones viejas sí caducan: el versionado está para deshacer un error, no para
  # guardar la historia dos veces.
  rule {
    id     = "versiones-viejas"
    status = "Enabled"
    filter {}
    noncurrent_version_expiration { noncurrent_days = 30 }
    abort_incomplete_multipart_upload { days_after_initiation = 7 }
  }

  # Los sobres solo caducan si se pide explícitamente. Un año de corridas de cinco pipelines
  # son unos pocos MB: borrarlos por defecto sería tirar la única evidencia que queda de un
  # fallo viejo para ahorrar centavos.
  dynamic "rule" {
    for_each = var.dias_retencion > 0 ? [1] : []
    content {
      id     = "sobres"
      status = "Enabled"
      filter { prefix = "sobres/" }
      expiration { days = var.dias_retencion }
    }
  }
}

output "nombre" { value = aws_s3_bucket.this.id }
output "arn" { value = aws_s3_bucket.this.arn }
