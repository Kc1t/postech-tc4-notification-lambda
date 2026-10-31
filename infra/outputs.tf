output "function_name" {
  value = aws_lambda_function.notifier.function_name
}

output "dedup_table" {
  value = aws_dynamodb_table.dedup.name
}
