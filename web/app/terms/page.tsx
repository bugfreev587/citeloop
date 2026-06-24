import Link from "next/link";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Terms of Service | CiteLoop",
  description: "Terms governing access to and use of CiteLoop.",
};

const updated = "June 24, 2026";

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-slate-200 py-8 first:border-t-0 first:pt-0">
      <h2 className="text-xl font-bold text-slate-950">{title}</h2>
      <div className="mt-4 space-y-4 text-[15px] leading-7 text-slate-600">{children}</div>
    </section>
  );
}

function BulletList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-2 pl-5">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}

export default function TermsPage() {
  return (
    <main className="min-h-[100dvh] bg-stone-100 px-4 py-10 text-slate-950">
      <article className="mx-auto max-w-3xl rounded-xl border border-slate-200 bg-white px-6 py-8 shadow-sm md:px-10">
        <Link href="/" className="text-sm font-semibold text-slate-500 hover:text-slate-950">
          CiteLoop
        </Link>
        <div className="mt-6">
          <p className="text-xs font-bold uppercase text-slate-400">Legal</p>
          <h1 className="mt-2 text-3xl font-bold text-slate-950">Terms of Service</h1>
          <p className="mt-2 text-sm text-slate-500">Last updated: {updated}</p>
        </div>

        <div className="mt-10">
          <Section title="1. Agreement to terms">
            <p>
              These Terms of Service ("Terms") are a legally binding agreement between you ("you" or "Customer") and
              CiteLoop ("CiteLoop", "we", "us", or "our") governing access to and use of the CiteLoop website,
              dashboard, API, automation workflows, and related services (collectively, the "Service").
            </p>
            <p>
              By accessing or using the Service, you agree to these Terms and to our{" "}
              <Link href="/privacy" className="font-semibold text-slate-900 underline decoration-slate-300">
                Privacy Policy
              </Link>
              . If you do not agree, you may not use the Service.
            </p>
          </Section>

          <Section title="2. Eligibility">
            <p>
              You must be at least 18 years old and have the legal authority to enter into these Terms. If you use the
              Service on behalf of an organization, you represent that you have authority to bind that organization.
            </p>
          </Section>

          <Section title="3. The Service">
            <p>
              CiteLoop helps teams analyze product domains, build SEO and GEO context, review content opportunities,
              generate evidence-backed drafts, publish or stage approved content, and measure outcomes.
            </p>
            <BulletList
              items={[
                "Project setup from a product domain.",
                "Public crawl, sitemap, robots, metadata, and evidence analysis.",
                "Google Search Console integration for customer-authorized first-party search data.",
                "Content planning, draft generation, review gates, publishing support, and results diagnostics.",
                "Optional publisher, CMS, notification, and analytics integrations as they become available.",
              ]}
            />
          </Section>

          <Section title="4. Accounts and security">
            <p>
              You may need an account to use the Service. You agree to provide accurate information, keep credentials
              secure, and promptly notify us of unauthorized access. You are responsible for activity under your account,
              workspace, API keys, and connected integrations.
            </p>
          </Section>

          <Section title="5. Connected services">
            <p>
              You may choose to connect third-party services such as Google Search Console, publisher platforms, CMS
              tools, repositories, notification providers, or billing providers. Your use of third-party services is
              subject to their own terms and privacy policies.
            </p>
            <p>
              When you connect Google Search Console, you authorize CiteLoop to access the selected Search Console
              property data for your project. You are responsible for ensuring that you have authority to connect that
              property and use the resulting data inside CiteLoop.
            </p>
          </Section>

          <Section title="6. Customer content and permissions">
            <p>
              You retain ownership of your product content, prompts, drafts, source materials, published pages, and
              project data. You grant CiteLoop a limited license to process that content and connected data only as
              needed to provide, secure, troubleshoot, and improve the Service.
            </p>
            <p>
              You are responsible for reviewing content before publishing, confirming factual claims, and complying with
              laws, platform rules, brand guidelines, and third-party rights.
            </p>
          </Section>

          <Section title="7. Acceptable use">
            <p>You agree not to use the Service to:</p>
            <BulletList
              items={[
                "Violate laws, regulations, platform rules, or third-party rights.",
                "Bypass authentication, authorization, rate limits, robots rules, or security controls.",
                "Upload or transmit malware, spam, deceptive content, or unlawful content.",
                "Reverse engineer the Service or attempt to extract non-public source code.",
                "Use the Service to build a competing product with non-public CiteLoop information.",
                "Misrepresent your ownership or authorization for a domain, Search Console property, publisher account, or CMS.",
              ]}
            />
          </Section>

          <Section title="8. Plans, fees, and limits">
            <p>
              CiteLoop may offer free, trial, or paid plans with usage limits. Plan details may change over time. If paid
              billing is enabled, charges are processed through third-party payment providers, and you authorize recurring
              charges according to the selected plan.
            </p>
          </Section>

          <Section title="9. Disclaimers">
            <p>
              To the maximum extent permitted by law, the Service is provided "as is" and "as available" without
              warranties of any kind. CiteLoop does not guarantee search rankings, traffic growth, indexing, AI-answer
              citations, publishing success, or uninterrupted access to third-party services.
            </p>
          </Section>

          <Section title="10. Limitation of liability">
            <p>
              To the maximum extent permitted by law, CiteLoop will not be liable for indirect, incidental, special,
              consequential, exemplary, or punitive damages, including lost profits, lost revenue, lost data, loss of
              goodwill, or business interruption.
            </p>
            <p>
              Our total liability for all claims will not exceed the amount you paid to CiteLoop in the twelve months
              before the event giving rise to the claim, or one hundred U.S. dollars if you have not paid CiteLoop.
            </p>
          </Section>

          <Section title="11. Indemnification">
            <p>
              You agree to defend, indemnify, and hold harmless CiteLoop from claims, damages, liabilities, costs, and
              expenses arising from your use of the Service, your content, your connected services, your violation of
              these Terms, or your violation of third-party rights.
            </p>
          </Section>

          <Section title="12. Termination">
            <p>
              You may stop using the Service at any time. We may suspend or terminate access if we believe you violated
              these Terms, created risk for the Service or other users, or used the Service unlawfully. Upon termination,
              your right to use the Service stops immediately.
            </p>
          </Section>

          <Section title="13. Governing law">
            <p>
              These Terms are governed by the laws of the State of California, without regard to conflict-of-law rules.
              Any dispute will be brought in the state or federal courts located in San Francisco County, California.
            </p>
          </Section>

          <Section title="14. Changes to these terms">
            <p>
              We may update these Terms from time to time. We will update the "Last updated" date when changes become
              effective. Your continued use of the Service after changes become effective constitutes acceptance of the
              updated Terms.
            </p>
          </Section>

          <Section title="15. Contact">
            <p>
              Questions about these Terms can be sent to{" "}
              <a href="mailto:support@citeloop.app" className="font-semibold text-slate-900 underline decoration-slate-300">
                support@citeloop.app
              </a>
              .
            </p>
          </Section>
        </div>

        <div className="mt-8 flex gap-4 border-t border-slate-200 pt-5 text-sm font-semibold text-slate-600">
          <Link href="/privacy" className="hover:text-slate-950">
            Privacy
          </Link>
          <Link href="/" className="hover:text-slate-950">
            Home
          </Link>
        </div>
      </article>
    </main>
  );
}
