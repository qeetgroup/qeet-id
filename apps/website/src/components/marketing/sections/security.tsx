import { IconAuditLog, IconMfaShield } from "@qeetrix/ui/brand";
import { FileCheckIcon, GlobeIcon, LockIcon, ShieldCheckIcon } from "lucide-react";
import type { ComponentType } from "react";

import { Reveal, Stagger, StaggerItem } from "@/components/marketing/motion";
import { Section, SectionHeader } from "@/components/marketing/section";

type TrustItem = {
  icon: ComponentType<{ className?: string }>;
  title: string;
  body: string;
};

const items: TrustItem[] = [
  {
    icon: ShieldCheckIcon,
    title: "SOC 2 Type II & ISO 27001",
    body: "Independently audited controls for security, availability, and confidentiality — reports available under NDA.",
  },
  {
    icon: FileCheckIcon,
    title: "HIPAA & GDPR ready",
    body: "Sign a BAA on Enterprise, and process data under a GDPR-compliant DPA with EU sub-processor transparency.",
  },
  {
    icon: GlobeIcon,
    title: "Data residency you choose",
    body: "Host in the US, EU, or APAC — with custom regions and single-tenant isolation available on Enterprise.",
  },
  {
    icon: IconAuditLog,
    title: "Tamper-evident audit log",
    body: "A hash-chained, append-only audit trail with a /verify endpoint — proof of integrity most platforms don't ship.",
  },
  {
    icon: IconMfaShield,
    title: "Secure by default",
    body: "Refresh-token theft detection, per-account lockout, phishing-resistant passkeys, and a production boot-gate.",
  },
  {
    icon: LockIcon,
    title: "Encrypted end to end",
    body: "TLS 1.3 in transit, AES-256 at rest, envelope-encrypted secrets, and per-tenant data isolation.",
  },
];

export function Security() {
  return (
    <Section id="security">
      <SectionHeader
        eyebrow="Security & Compliance"
        title="Trusted with your"
        titleAccent="most sensitive data"
        subtitle="Identity is the front door to everything. Qeet ID is built to pass the security review — and give your customers' security teams less to worry about."
      />

      <Stagger
        staggerDelay={0.06}
        className="mt-14 grid auto-rows-fr grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3"
      >
        {items.map((item) => {
          const Icon = item.icon;
          return (
            <StaggerItem key={item.title} className="h-full">
              <div className="flex h-full flex-col gap-3 rounded-2xl border border-border/60 bg-background p-6">
                <span className="grid size-10 place-items-center rounded-lg bg-brand/10 text-brand ring-1 ring-brand/20">
                  <Icon className="size-5" />
                </span>
                <h3 className="font-display text-base font-semibold tracking-tight">{item.title}</h3>
                <p className="text-sm text-muted-foreground">{item.body}</p>
              </div>
            </StaggerItem>
          );
        })}
      </Stagger>

      <Reveal className="mt-8 flex flex-wrap items-center justify-center gap-x-6 gap-y-2 text-xs font-medium uppercase tracking-widest text-muted-foreground">
        <span>SOC 2 Type II</span>
        <span aria-hidden>·</span>
        <span>ISO 27001</span>
        <span aria-hidden>·</span>
        <span>HIPAA BAA</span>
        <span aria-hidden>·</span>
        <span>GDPR</span>
        <span aria-hidden>·</span>
        <span>FIDO2 / WebAuthn</span>
      </Reveal>
    </Section>
  );
}
