import Link from "next/link";
import { ArrowRightIcon } from "lucide-react";

import { ButtonLink } from "@/components/marketing/button-link";
import { Reveal, Stagger, StaggerItem } from "@/components/marketing/motion";
import { Section, SectionHeader } from "@/components/marketing/section";

/**
 * Homepage compare teaser — links out to the full, verified head-to-head
 * comparison pages under `/compare/{slug}`. Kept intentionally light (no
 * comparison table) so the homepage stays scannable.
 */
const competitors = [
  { slug: "auth0", name: "Auth0", category: "Enterprise CIAM" },
  { slug: "clerk", name: "Clerk", category: "Developer-first auth" },
  { slug: "workos", name: "WorkOS", category: "Enterprise-readiness APIs" },
  { slug: "stytch", name: "Stytch", category: "Passwordless + fraud" },
];

export function Compare() {
  return (
    <Section id="compare">
      <SectionHeader
        eyebrow="Compare"
        title="How Qeet ID"
        titleAccent="stacks up"
        subtitle="Honest, side-by-side comparisons against the platforms you've probably already evaluated — verified against what we actually ship, gaps included."
      />

      <Stagger staggerDelay={0.08} className="mt-14 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {competitors.map((c) => (
          <StaggerItem key={c.slug} className="h-full">
            <Link
              href={`/compare/${c.slug}`}
              className="group flex h-full flex-col gap-3 rounded-2xl border border-border/60 bg-card p-6 transition-colors hover:border-brand/50 focus-ring-brand"
            >
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs font-medium uppercase tracking-widest text-brand-text">
                  {c.category}
                </span>
                <ArrowRightIcon className="size-4 text-muted-foreground transition-transform group-hover:translate-x-1 group-hover:text-foreground" />
              </div>
              <h3 className="font-display text-lg font-semibold tracking-tight">
                Qeet ID <span className="text-muted-foreground">vs.</span> {c.name}
              </h3>
            </Link>
          </StaggerItem>
        ))}
      </Stagger>

      <Reveal className="mt-10 text-center">
        <ButtonLink variant="ghost" href="/compare">
          See all comparisons <ArrowRightIcon className="size-4" />
        </ButtonLink>
      </Reveal>
    </Section>
  );
}
