# registry-inventory-generator
A tool to generate report on docker registry usage (images, tags, size,date of upload etc)
# Usage:
- Setup below environment variables
 - REGISTRY_HOST (url for your docker private registry ex:registry.docker.com ,https://registry.docker.excom )
 - REGISTRY_USERNAME
 - REGISTRY_PASSWORD
 - REGISTRY_RETENTION (To keep these many latest tags for each image )
 - REGISTRY_WORKER (Number og go routines to run)
 - Run ./registry-inventory-generator
   (Be patient ,it will take time to go through each and every image/tags and pull the details) 
- Output:
   Output file will be created with below format
   ${registry_name}.reports.json
- Logs:
  Logs will be available under registry_reports.log

# Development
 - Tested with go version 1.16.4
 - clone the repo and "go build"  
