variable "bucket" { type = string }

# El panel es un sitio estático PRIVADO: solo CloudFront lo lee, vía OAC. Nada de "website
# hosting" con bucket público — eso obligaría a exponer el bucket y perdería HTTPS.
resource "aws_s3_bucket" "this" {
  bucket = var.bucket
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

output "id" { value = aws_s3_bucket.this.id }
output "nombre" { value = aws_s3_bucket.this.id }
output "arn" { value = aws_s3_bucket.this.arn }
output "dominio_regional" { value = aws_s3_bucket.this.bucket_regional_domain_name }
