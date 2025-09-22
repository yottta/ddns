# DDNS CloudFlare

This is a small tool to run inside your home lab that updates a record with the currently allocated IP from ISP.

This tool requires 3 environment variables:
* CLOUDFLARE_API_TOKEN - used by the cloudflare sdk to authenticate the requests. This needs to have read/write permissions on your dns entries
* CLOUDFLARE_ZONEID - the cloudflare zoneId where the dns entries can be found
* CLOUDFLARE_ENTRY_ID - the cloudflare dns entry to check and update

To determine the currently ISP assigned IP, this tool calls https://www.mapper.ntppool.org/json.