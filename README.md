# registry-inventory-generator
A tool to generate report on docker registry usage (images, tags, size,date of upload etc)
Usage:
- setup below environment variables
 - REGISTRY (ex:https://registry.docker.com)
 - REGISTRY_USERNAME
 - REGISTRY_PASSWORD
- run ./registry-inventory-generator
(Be patient ,it will take time to go through each and every image/tags and pull the details) 
-- Output:
   Output file will be created with below format
   registry_name.reports.json
--- Logs:
Logs will be available under /var/logs/registry_reports.log
