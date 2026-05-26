import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  DataState,
  StatusPill,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TimeSince,
} from "@qeetid/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { MonitorSmartphoneIcon } from "lucide-react";

import { api, tokenStore } from "@/lib/api";

export const Route = createFileRoute("/account/sessions")({ component: SessionsPage });

type Session = {
  id: string;
  user_id: string;
  tenant_id: string;
  ip?: string | null;
  user_agent?: string | null;
  created_at: string;
  last_seen_at: string;
  revoked_at?: string | null;
};

function SessionsPage() {
  const userId = tokenStore.getUserId();
  const qc = useQueryClient();

  const sessionsQ = useQuery({
    queryKey: ["account-sessions", userId],
    queryFn: () => api<{ items: Session[] }>(`/v1/users/${userId}/sessions`),
    enabled: !!userId,
  });

  const revokeM = useMutation({
    mutationFn: (id: string) => api<void>(`/v1/sessions/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["account-sessions"] }),
    meta: { successMessage: "Session revoked" },
  });

  const items = sessionsQ.data?.items ?? [];
  const active = items.filter((s) => !s.revoked_at);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Active sessions</CardTitle>
        <CardDescription>
          Every place you&apos;re currently signed in. Revoke any session you don&apos;t
          recognise.
        </CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <DataState
          isLoading={sessionsQ.isLoading}
          isError={sessionsQ.isError}
          error={sessionsQ.error}
          isEmpty={active.length === 0}
          emptyIcon={MonitorSmartphoneIcon}
          emptyTitle="No active sessions."
          skeletonRows={3}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Device</TableHead>
                <TableHead>IP</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Last seen</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((s) => (
                <TableRow key={s.id}>
                  <TableCell
                    className="max-w-md truncate text-xs text-muted-foreground"
                    title={s.user_agent ?? ""}
                  >
                    <MonitorSmartphoneIcon className="mr-1 inline size-3" />
                    {s.user_agent ?? "—"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {s.ip ?? "—"}
                  </TableCell>
                  <TableCell>
                    <TimeSince value={s.created_at} />
                  </TableCell>
                  <TableCell>
                    <TimeSince value={s.last_seen_at} />
                  </TableCell>
                  <TableCell>
                    <StatusPill status={s.revoked_at ? "revoked" : "active"} />
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!!s.revoked_at || revokeM.isPending}
                      onClick={() => {
                        if (confirm("Revoke this session? Whoever holds it will be signed out.")) {
                          revokeM.mutate(s.id);
                        }
                      }}
                    >
                      Revoke
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>
  );
}
