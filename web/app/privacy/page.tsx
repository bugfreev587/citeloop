import Link from "next/link";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Privacy Policy | CiteLoop",
  description: "How CiteLoop collects, uses, protects, and limits data for SEO and GEO workflows.",
};

const updated = "June 24, 2026";

const summary = [
  "We collect only the information needed to run CiteLoop projects, analyze public websites, and operate customer-authorized integrations.",
  "When you connect Google Search Console, CiteLoop requests the minimum Search Console scope needed for first-party search analysis.",
  "OAuth access tokens and refresh tokens are stored server-side and protected as secrets. We do not ask for or store Google passwords.",
  "We do not sell personal data and do not use Google user data for advertising, retargeting, credit decisions, or unrelated model training.",
  "You can disconnect integrations or request deletion of your account data by contacting support@citeloop.app.",
];

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

export default function PrivacyPage() {
  return (
    <main className="min-h-[100dvh] bg-stone-100 px-4 py-10 text-slate-950">
      <article className="mx-auto max-w-3xl rounded-xl border border-slate-200 bg-white px-6 py-8 shadow-sm md:px-10">
        <Link href="/" className="text-sm font-semibold text-slate-500 hover:text-slate-950">
          CiteLoop
        </Link>
        <div className="mt-6">
          <p className="text-xs font-bold uppercase text-slate-400">Legal</p>
          <h1 className="mt-2 text-3xl font-bold text-slate-950">Privacy Policy</h1>
          <p className="mt-2 text-sm text-slate-500">Last updated: {updated}</p>
        </div>

        <div className="mt-8 space-y-1 rounded-lg border border-emerald-100 bg-emerald-50 p-4 text-sm leading-6 text-emerald-950">
          <div className="font-bold">Summary of key points</div>
          <BulletList items={summary} />
        </div>

        <div className="mt-10">
          <Section title="1. Information we collect">
            <p>
              CiteLoop ("CiteLoop", "we", "us", or "our") collects information when you access{" "}
              <Link href="https://citeloop.app" className="font-semibold text-slate-900 underline decoration-slate-300">
                https://citeloop.app
              </Link>
              , create a project, connect integrations, or use the CiteLoop dashboard, API, and related services.
            </p>
            <BulletList
              items={[
                "Account information such as name, email address, authentication identifiers, workspace membership, and account preferences.",
                "Project information such as product domains, crawl settings, brand voice, content plans, review decisions, publisher settings, and notification settings.",
                "Public website data such as sitemap URLs, robots rules, page titles, metadata, visible page copy, internal links, and crawl status.",
                "Usage and operational data such as API request logs, dashboard actions, automation events, errors, and billing or plan status.",
              ]}
            />
          </Section>

          <Section title="2. Google Search Console data">
            <p>
              If you connect Google Search Console, CiteLoop accesses Google Search Console data only for properties
              you authorize and select inside CiteLoop. This may include Search Console property identifiers, verified
              site URLs, query data, page data, impressions, clicks, click-through rate, average position, indexing
              diagnostics, sitemap status, and related metadata returned by the Search Console API.
            </p>
            <p>
              CiteLoop stores OAuth access tokens and refresh tokens so the service can refresh authorization and
              update project analysis without asking you to reconnect on every run. Tokens are stored server-side and
              protected as credentials. We do not store Google passwords.
            </p>
            <p>
              Our use and transfer of information received from Google APIs adheres to the{" "}
              <Link
                href="https://developers.google.com/terms/api-services-user-data-policy"
                className="font-semibold text-slate-900 underline decoration-slate-300"
              >
                Google API Services User Data Policy
              </Link>
              , including the Limited Use requirements.
            </p>
          </Section>

          <Section title="3. How we use information">
            <BulletList
              items={[
                "Provide, operate, secure, and improve the CiteLoop service.",
                "Build domain context, identify SEO and GEO opportunities, prioritize analysis, and generate content plans.",
                "Use Google Search Console data to show query, CTR, position, page, and content-decay signals inside Analysis and Results.",
                "Prepare evidence-backed drafts, review queues, publishing checks, and measurement diagnostics.",
                "Send service notifications, enforce usage limits, troubleshoot errors, and prevent abuse.",
              ]}
            />
          </Section>

          <Section title="4. Sharing of information">
            <p>We share information only as needed to provide the service, comply with law, or protect users.</p>
            <BulletList
              items={[
                "Service providers such as Clerk for authentication, Vercel for web hosting, Railway or equivalent infrastructure providers for API hosting, and payment providers if paid billing is enabled.",
                "Google APIs when you choose to connect Google Search Console and authorize CiteLoop to access your selected properties.",
                "Publisher or CMS providers only when you connect them and instruct CiteLoop to publish, draft, or verify content.",
                "Legal, security, or compliance recipients when required by law or necessary to investigate abuse.",
              ]}
            />
            <p>We do not sell personal data and do not share Google user data with advertisers.</p>
          </Section>

          <Section title="5. Google data restrictions">
            <BulletList
              items={[
                "We use Google Search Console data only to provide or improve user-facing CiteLoop features shown in the dashboard.",
                "We do not use Google user data for advertising, retargeting, personalized ads, credit decisions, or unrelated profiling.",
                "We do not transfer Google user data to third parties except as necessary to provide CiteLoop features you requested, comply with law, or protect security.",
                "We do not allow humans to inspect raw Google user data unless needed for security, legal compliance, debugging with your permission, or aggregated internal operations.",
              ]}
            />
          </Section>

          <Section title="6. Cookies and tracking">
            <p>
              CiteLoop uses essential cookies and similar technologies for authentication, session management, and
              security. We do not use advertising cookies or cross-site behavioral advertising trackers.
            </p>
          </Section>

          <Section title="7. Data retention and deletion">
            <BulletList
              items={[
                "Account and project data is retained while your account remains active.",
                "OAuth tokens are retained while an integration remains connected and are deleted or revoked when you disconnect the integration where supported.",
                "Operational logs are retained for a limited period for security, debugging, and reliability.",
                "Upon account deletion, personal data and connected credentials are deleted within a reasonable period unless retention is required by law.",
              ]}
            />
          </Section>

          <Section title="8. Security">
            <p>
              We use reasonable technical and organizational safeguards, including HTTPS, access controls, credential
              protection, audit logs, and secret isolation. No system is perfectly secure, but we work to protect
              customer data against unauthorized access, disclosure, alteration, or destruction.
            </p>
          </Section>

          <Section title="9. Your choices and rights">
            <BulletList
              items={[
                "You can disconnect Google Search Console or other integrations from the dashboard when available.",
                "You can revoke Google access from your Google Account security settings.",
                "You can request access, correction, export, or deletion of your personal data by contacting us.",
              ]}
            />
          </Section>

          <Section title="10. Children's privacy">
            <p>
              CiteLoop is not directed to children under 16, and we do not knowingly collect personal information from children.
            </p>
          </Section>

          <Section title="11. Changes to this policy">
            <p>
              We may update this Privacy Policy from time to time. We will update the "Last updated" date when changes
              become effective. Material changes may also be communicated through the dashboard or email.
            </p>
          </Section>

          <Section title="12. Contact">
            <p>
              Questions about this Privacy Policy can be sent to{" "}
              <a href="mailto:support@citeloop.app" className="font-semibold text-slate-900 underline decoration-slate-300">
                support@citeloop.app
              </a>
              .
            </p>
          </Section>
        </div>

        <div className="mt-8 flex gap-4 border-t border-slate-200 pt-5 text-sm font-semibold text-slate-600">
          <Link href="/terms" className="hover:text-slate-950">
            Terms
          </Link>
          <Link href="/" className="hover:text-slate-950">
            Home
          </Link>
        </div>
      </article>
    </main>
  );
}
