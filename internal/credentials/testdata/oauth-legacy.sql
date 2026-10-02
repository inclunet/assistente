INSERT INTO mcp_servers (id,created_at,updated_at,user_id,slug,name,transport,url,auth_type,o_auth2_client_id,o_auth2_auth_url,o_auth2_token_url,o_auth2_scopes,o_auth2_callback_port,o_auth2_callback_host,o_auth2_registration_url,o_auth2_device_auth_url,prefer_bridge)
VALUES ('fixture-server','2026-09-22 00:00:00+00:00','2026-09-22 00:00:00+00:00','018f0000-0000-7000-8000-000000000001','published','Published fixture','streamable','https://resource.example/mcp','oauth2_pkce','fixture-client','https://identity.example/authorize','https://identity.example/token','["read","offline_access"]',3128,'localhost','https://identity.example/register','https://identity.example/device',true);
INSERT INTO credential_entries (id,user_id,pattern,auth_type,client_id_enc,client_secret_enc)
VALUES ('fixture-client-row','018f0000-0000-7000-8000-000000000001','mcp-client:published','oauth2','EkxYQueteaLE7DwkgrCjYK+Bn2h+MKsN8Ho5AQ7wuDgYdOhiQBCmxoz3','A9lKaRtXMhR+zxLyRArWpd/WC3P8sVh0H7y0PHfsEih8DF4svjVQGjEl');
INSERT INTO credential_entries (id,user_id,pattern,auth_type,token_enc,refresh_token_enc,expires_at)
VALUES ('fixture-token-row','018f0000-0000-7000-8000-000000000001','mcp-tokens:published','oauth2','lBMUXxJ+vtxeStfGpiacCfqPj+nE62I1sz/r3lKHWbJPte3j5+fpj34t','3LabMn5SFdkZczsiXWdYT95/jnxrDNg1EfVbaAS9jBJR5ri8+xAZpAXSZw==',1790038800);
INSERT INTO credential_entries (id,user_id,pattern,auth_type,token_enc)
VALUES ('fixture-host-row','018f0000-0000-7000-8000-000000000001','resource.example','bearer','JwkVBVacUzgiqs8VQ1vwEq84okfDF4mJbBI/As8swLoHmGZhYQIANM9hUto0yQ==');
INSERT INTO credential_entries (id,user_id,pattern,auth_type,token_enc)
VALUES ('fixture-other-row','018f0000-0000-7000-8000-000000000002','mcp-tokens:published','oauth2','lBMUXxJ+vtxeStfGpiacCfqPj+nE62I1sz/r3lKHWbJPte3j5+fpj34t');

