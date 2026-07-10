// Self-serve Admin Portal client. Every call is scoped by the {token} path
// segment — no cookie session, no bearer JWT. See the Go package
// domains/federation/adminportal for the server-side contract.

import { apiDelete, apiGet, apiPatch, apiPost } from "./api";
import type { BrandingDTO } from "./branding";

export type AdminPortalCapability = "saml" | "scim";

export interface PortalContext {
  tenant_name: string;
  capabilities: AdminPortalCapability[];
  expires_at: string;
  branding?: BrandingDTO;
}

export type SamlStatus = "draft" | "active" | "disabled";

export interface SamlConnection {
  id: string;
  tenant_id: string;
  name: string;
  idp_entity_id: string;
  idp_sso_url: string;
  idp_certificate: string;
  email_attribute: string;
  name_attribute: string;
  status: SamlStatus;
  created_at: string;
  updated_at: string;
  last_login_at: string | null;
}

export interface SamlConnectionInput {
  name?: string;
  idp_entity_id?: string;
  idp_sso_url?: string;
  idp_certificate?: string;
  email_attribute?: string;
  name_attribute?: string;
  status?: SamlStatus;
}

export interface SamlTestResult {
  ok: boolean;
  checks: { name: string; ok: boolean; detail?: string }[];
}

export interface ScimConfig {
  token_set: boolean;
  token_prefix?: string;
  created_at?: string | null;
  last_used_at?: string | null;
  provisioned_count: number;
}

export function fetchPortalContext(token: string) {
  return apiGet<PortalContext>(`/v1/admin-portal/${token}/context`);
}

export function listSamlConnections(token: string) {
  return apiGet<{ items: SamlConnection[] }>(`/v1/admin-portal/${token}/saml`);
}

export function createSamlConnection(token: string, input: SamlConnectionInput) {
  return apiPost<SamlConnection>(`/v1/admin-portal/${token}/saml`, input);
}

export function updateSamlConnection(token: string, id: string, input: SamlConnectionInput) {
  return apiPatch<SamlConnection>(`/v1/admin-portal/${token}/saml/${id}`, input);
}

export function testSamlConnection(token: string, id: string) {
  return apiPost<SamlTestResult>(`/v1/admin-portal/${token}/saml/${id}/test`);
}

export function deleteSamlConnection(token: string, id: string) {
  return apiDelete<void>(`/v1/admin-portal/${token}/saml/${id}`);
}

export function getScimConfig(token: string) {
  return apiGet<ScimConfig>(`/v1/admin-portal/${token}/scim`);
}

export function rotateScimToken(token: string) {
  return apiPost<{ token: string; config: ScimConfig }>(`/v1/admin-portal/${token}/scim/token`);
}

export function revokeScimToken(token: string) {
  return apiDelete<void>(`/v1/admin-portal/${token}/scim/token`);
}
