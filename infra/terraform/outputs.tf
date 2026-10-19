output "api_url" {
  value       = module.lambda.function_url
  description = "URL directa de la API. Es la que va en ETL_LOGS_URL de cada pipeline, con /api/v1/corridas al final."
}

output "panel_url" {
  value       = module.cloudfront.dominio
  description = "El panel."
}

output "ecr_url" {
  value       = module.ecr.url
  description = "Adónde empujar la imagen."
}

output "bucket_datos" { value = module.s3_datos.nombre }
output "bucket_web" { value = module.s3_web.nombre }

output "siguiente_paso" {
  value = <<-TXT
    1. Cargar los secretos (no van en el tfstate):
         aws ssm put-parameter --name ${module.ssm.prefijo}/token_emisor --type SecureString --value '...' --overwrite
         aws ssm put-parameter --name ${module.ssm.prefijo}/token_lector --type SecureString --value '...' --overwrite
    2. Empujar la imagen:
         docker build --target lambda -t ${module.ecr.url}:latest backend/
         aws ecr get-login-password | docker login --username AWS --password-stdin ${module.ecr.url}
         docker push ${module.ecr.url}:latest
         aws lambda update-function-code --function-name ${module.lambda.nombre} --image-uri ${module.ecr.url}:latest
    3. Subir el panel:
         NEXT_PUBLIC_API_URL=${module.lambda.function_url}api/v1 npm --prefix frontend run build
         aws s3 sync frontend/out/ s3://${module.s3_web.nombre}/ --delete
  TXT
}
