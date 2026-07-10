import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Field,
  FieldDescription,
  FieldLabel,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@qeetrix/ui";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon, PlaneIcon, ShieldCheckIcon, SmartphoneIcon } from "lucide-react";
import { useEffect, useState } from "react";

import { PageHeader } from "@/components/page-header";
import { api } from "@/lib/api";
import { useTenantId } from "@/lib/auth";

export const Route = createFileRoute("/_app/security/threats/risk-settings")({
  component: RiskSettingsPage,
});

interface RiskSettings {
  medium_threshold: number;
  high_threshold: number;
  force_mfa_at_level: "medium" | "high";
  impossible_travel_enabled: boolean;
  min_travel_hours: number;
  device_reputation_enabled: boolean;
}

function useRiskSettings() {
  const tenantId = useTenantId();
  return useQuery({
    queryKey: ["risk-settings", tenantId],
    queryFn: () => api<RiskSettings>(`/v1/tenants/${tenantId}/security/risk-settings`),
    enabled: !!tenantId,
  });
}

function useUpdateRiskSettings() {
  const tenantId = useTenantId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RiskSettings) =>
      api<RiskSettings>(`/v1/tenants/${tenantId}/security/risk-settings`, {
        method: "PUT",
        body,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["risk-settings", tenantId] }),
  });
}

function RiskSettingsPage() {
  const settingsQ = useRiskSettings();
  const update = useUpdateRiskSettings();

  const [medium, setMedium] = useState(0.5);
  const [high, setHigh] = useState(0.8);
  const [forceAt, setForceAt] = useState<"medium" | "high">("high");
  const [travelEnabled, setTravelEnabled] = useState(false);
  const [minTravelHours, setMinTravelHours] = useState(3);
  const [deviceEnabled, setDeviceEnabled] = useState(false);

  useEffect(() => {
    if (settingsQ.data) {
      setMedium(settingsQ.data.medium_threshold);
      setHigh(settingsQ.data.high_threshold);
      setForceAt(settingsQ.data.force_mfa_at_level);
      setTravelEnabled(settingsQ.data.impossible_travel_enabled);
      setMinTravelHours(settingsQ.data.min_travel_hours);
      setDeviceEnabled(settingsQ.data.device_reputation_enabled);
    }
  }, [settingsQ.data]);

  const dirty =
    settingsQ.data &&
    (medium !== settingsQ.data.medium_threshold ||
      high !== settingsQ.data.high_threshold ||
      forceAt !== settingsQ.data.force_mfa_at_level ||
      travelEnabled !== settingsQ.data.impossible_travel_enabled ||
      minTravelHours !== settingsQ.data.min_travel_hours ||
      deviceEnabled !== settingsQ.data.device_reputation_enabled);

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <PageHeader description="Configure adaptive MFA. When a login's risk score — bot-likeness, plus impossible-travel and device-reputation if you enable them — exceeds your chosen threshold, MFA is required even on a remembered device." />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheckIcon className="size-4" />
            Risk Thresholds
          </CardTitle>
          <CardDescription>
            Bot scores range from 0 (clearly human) to 1 (clearly automated). A score above the
            High threshold forces MFA even on a trusted device.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {settingsQ.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : (
            <form
              className="flex flex-col gap-5"
              onSubmit={(e) => {
                e.preventDefault();
                update.mutate({
                  medium_threshold: medium,
                  high_threshold: high,
                  force_mfa_at_level: forceAt,
                  impossible_travel_enabled: travelEnabled,
                  min_travel_hours: minTravelHours,
                  device_reputation_enabled: deviceEnabled,
                });
              }}
            >
              <div className="grid gap-4 sm:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor="medium">Medium threshold (0.1–1.0)</FieldLabel>
                  <Input
                    id="medium"
                    type="number"
                    step="0.05"
                    min={0.1}
                    max={1.0}
                    value={medium}
                    onChange={(e) => setMedium(Number(e.target.value))}
                  />
                  <FieldDescription>
                    Score at/above this triggers a step-up MFA challenge for unrecognised devices.
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="high">High threshold (0.1–1.0)</FieldLabel>
                  <Input
                    id="high"
                    type="number"
                    step="0.05"
                    min={0.1}
                    max={1.0}
                    value={high}
                    onChange={(e) => setHigh(Number(e.target.value))}
                  />
                  <FieldDescription>
                    Score at/above this forces MFA even on a trusted/remembered device.
                  </FieldDescription>
                </Field>
              </div>

              <Field className="max-w-xs">
                <FieldLabel htmlFor="force-at">Force MFA at level</FieldLabel>
                <Select value={forceAt} onValueChange={(v) => setForceAt(v as "medium" | "high")}>
                  <SelectTrigger id="force-at">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="medium">Medium — force on any suspicious request</SelectItem>
                    <SelectItem value="high">High — force only on clearly automated requests</SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>
                  Which risk level overrides the trusted-device skip.
                </FieldDescription>
              </Field>

              <div className="flex flex-col gap-4 border-t pt-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-start gap-2">
                    <PlaneIcon className="mt-0.5 size-4 text-muted-foreground" />
                    <div>
                      <p className="text-sm font-medium">Impossible travel</p>
                      <p className="text-sm text-muted-foreground">
                        Flag a login from a new country too soon after the last one to be
                        plausible travel. Off by default — also needs a trusted upstream proxy
                        to supply a country header (see docs); with none configured this never
                        fires.
                      </p>
                    </div>
                  </div>
                  <Switch checked={travelEnabled} onCheckedChange={setTravelEnabled} />
                </div>
                {travelEnabled && (
                  <Field className="max-w-xs pl-6">
                    <FieldLabel htmlFor="min-travel-hours">Minimum plausible travel time (hours)</FieldLabel>
                    <Input
                      id="min-travel-hours"
                      type="number"
                      step="0.5"
                      min={0.5}
                      value={minTravelHours}
                      onChange={(e) => setMinTravelHours(Number(e.target.value))}
                    />
                  </Field>
                )}

                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-start gap-2">
                    <SmartphoneIcon className="mt-0.5 size-4 text-muted-foreground" />
                    <div>
                      <p className="text-sm font-medium">Device reputation</p>
                      <p className="text-sm text-muted-foreground">
                        Flag a login from a browser + OS combination never seen before for that
                        user. Off by default.
                      </p>
                    </div>
                  </div>
                  <Switch checked={deviceEnabled} onCheckedChange={setDeviceEnabled} />
                </div>
              </div>

              <div className="flex items-center gap-3">
                <Button type="submit" disabled={!dirty || update.isPending}>
                  {update.isPending && <Loader2Icon className="animate-spin" />}
                  Save changes
                </Button>
                {update.isSuccess && (
                  <span className="text-sm text-green-600">Saved.</span>
                )}
              </div>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
