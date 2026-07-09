import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  DataState,
  Field,
  FieldLabel,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TimeSince,
} from "@qeetrix/ui";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  AlertTriangleIcon,
  BrainCircuitIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  SparklesIcon,
} from "lucide-react";
import { useEffect, useState } from "react";

import { PageHeader } from "@/components/page-header";
import { ApiError } from "@/lib/api";
import {
  useAuditAnomalies,
  useAuditAnomalySettings,
  useAuditAnomalySummary,
  useResolveAuditAnomaly,
  useUpdateAuditAnomalySettings,
  useVerifyAuditChain,
  type AnomalyReason,
} from "@/lib/audit-anomalies";

export const Route = createFileRoute("/_app/security/audit-intelligence")({
  component: AuditIntelligencePage,
});

const reasonLabel: Record<AnomalyReason, string> = {
  new_action_type: "Never done this before",
  unusual_hour: "Unusual time of day",
  new_ip: "New IP address",
};

function scoreBadge(score: number) {
  if (score >= 0.85) return <Badge variant="destructive">{score.toFixed(2)}</Badge>;
  if (score >= 0.7) return <Badge variant="secondary">{score.toFixed(2)}</Badge>;
  return <Badge variant="outline">{score.toFixed(2)}</Badge>;
}

function AuditIntelligencePage() {
  const [status, setStatus] = useState<"open" | "resolved">("open");
  const anomaliesQ = useAuditAnomalies(status);
  const summaryQ = useAuditAnomalySummary();
  const resolve = useResolveAuditAnomaly();
  const items = anomaliesQ.data?.items ?? [];
  const sm = summaryQ.data;

  const summary = [
    { label: "Open", value: sm?.open ?? 0, icon: <AlertTriangleIcon className="size-4" /> },
    {
      label: "High score (open)",
      value: sm?.high_score_open ?? 0,
      icon: <SparklesIcon className="size-4" />,
    },
    {
      label: "Resolved (7d)",
      value: sm?.resolved_7d ?? 0,
      icon: <ShieldCheckIcon className="size-4" />,
    },
  ];

  return (
    <div className="flex min-w-0 flex-col gap-6">
      <PageHeader
        description="Behavioral-baseline anomaly detection over the audit log: a first-time action type, an unusual hour, or a new IP for an admin's own history. Distinct from Threat Protection's Anomalies, which watches login/session signals."
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              anomaliesQ.refetch();
              summaryQ.refetch();
            }}
            disabled={anomaliesQ.isFetching}
          >
            <RefreshCwIcon className={anomaliesQ.isFetching ? "animate-spin" : ""} />
            Refresh
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {summary.map((s) => (
          <Card key={s.label}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardDescription>{s.label}</CardDescription>
              <span className="text-muted-foreground">{s.icon}</span>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-semibold tracking-tight">{s.value}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4">
          <div>
            <CardTitle>Anomalies</CardTitle>
            <CardDescription>Scored by a background sweep, newest first.</CardDescription>
          </div>
          <Select value={status} onValueChange={(v) => v && setStatus(v as "open" | "resolved")}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="open">Open</SelectItem>
              <SelectItem value="resolved">Resolved</SelectItem>
            </SelectContent>
          </Select>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <DataState
            isLoading={anomaliesQ.isLoading}
            isError={anomaliesQ.isError}
            error={anomaliesQ.error}
            isEmpty={items.length === 0}
            emptyIcon={BrainCircuitIcon}
            emptyTitle={status === "open" ? "No open anomalies." : "Nothing resolved yet."}
            skeletonRows={3}
          >
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Why it's flagged</TableHead>
                  <TableHead>Score</TableHead>
                  <TableHead>When</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell className="font-mono text-xs">{a.action}</TableCell>
                    <TableCell>
                      {a.actor_user_id ? (
                        <Link
                          to="/security/audit-logs"
                          search={{ actor_user_id: a.actor_user_id }}
                          className="underline"
                        >
                          {a.actor_email ?? a.actor_user_id}
                        </Link>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {a.reasons.map((r) => reasonLabel[r] ?? r).join(", ")}
                    </TableCell>
                    <TableCell>{scoreBadge(a.score)}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      <TimeSince value={a.event_at} />
                    </TableCell>
                    <TableCell className="text-right">
                      {a.status === "resolved" ? (
                        <span className="text-xs text-muted-foreground">Resolved</span>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={resolve.isPending}
                          onClick={() => resolve.mutate(a.id)}
                        >
                          Resolve
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      <SettingsCard />
      <VerifyCard />
    </div>
  );
}

function SettingsCard() {
  const settingsQ = useAuditAnomalySettings();
  const updateM = useUpdateAuditAnomalySettings();
  const [enabled, setEnabled] = useState(true);
  const [threshold, setThreshold] = useState(0.6);
  const [minEvents, setMinEvents] = useState(20);

  useEffect(() => {
    if (settingsQ.data) {
      setEnabled(settingsQ.data.enabled);
      setThreshold(settingsQ.data.score_threshold);
      setMinEvents(settingsQ.data.min_baseline_events);
    }
  }, [settingsQ.data]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <div>
          <CardTitle className="text-base">Detection settings</CardTitle>
          <CardDescription>Tune sensitivity for this tenant.</CardDescription>
        </div>
        <Switch checked={enabled} onCheckedChange={setEnabled} />
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <Field className="sm:w-48">
            <FieldLabel htmlFor="threshold">Score threshold</FieldLabel>
            <Input
              id="threshold"
              type="number"
              min={0}
              max={1}
              step={0.05}
              value={threshold}
              onChange={(e) => setThreshold(Number(e.target.value))}
            />
          </Field>
          <Field className="sm:w-48">
            <FieldLabel htmlFor="min-events">Cold-start guard (events)</FieldLabel>
            <Input
              id="min-events"
              type="number"
              min={0}
              value={minEvents}
              onChange={(e) => setMinEvents(Number(e.target.value))}
            />
          </Field>
          <Button
            disabled={updateM.isPending}
            onClick={() =>
              updateM.mutate({
                enabled,
                score_threshold: threshold,
                min_baseline_events: minEvents,
              })
            }
          >
            Save
          </Button>
        </div>
        {updateM.error && (
          <p className="text-destructive text-sm">{(updateM.error as ApiError).message}</p>
        )}
      </CardContent>
    </Card>
  );
}

function VerifyCard() {
  const verifyM = useVerifyAuditChain();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Chain integrity</CardTitle>
        <CardDescription>
          Walk the tenant's hash-chained audit log and confirm no row has been tampered with.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div>
          <Button variant="outline" disabled={verifyM.isPending} onClick={() => verifyM.mutate()}>
            {verifyM.isPending ? "Verifying…" : "Verify integrity"}
          </Button>
        </div>
        {verifyM.data && (
          <p className={`text-sm ${verifyM.data.ok ? "text-muted-foreground" : "text-destructive"}`}>
            {verifyM.data.ok
              ? `OK — ${verifyM.data.rows_checked} row(s) checked, chain intact.`
              : `Broken at row ${verifyM.data.broken_at_id}: ${verifyM.data.broken_reason}`}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
