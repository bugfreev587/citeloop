import type { SiteFix } from "./types";

type DoctorFindingReference = { id: string };

export type DoctorRecentFindingLink<TFinding extends DoctorFindingReference = DoctorFindingReference> = {
  finding: TFinding;
  siteFix: SiteFix;
};

function siteFixCreatedRank(siteFix: SiteFix) {
  const timestamp = Date.parse(siteFix.created_at ?? "");
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function isNewerSiteFix(candidate: SiteFix, current: SiteFix) {
  const timeDifference = siteFixCreatedRank(candidate) - siteFixCreatedRank(current);
  if (timeDifference !== 0) return timeDifference > 0;
  return candidate.id.localeCompare(current.id) > 0;
}

export function latestSiteFixByFinding(siteFixes: SiteFix[]) {
  const latest = new Map<string, SiteFix>();
  for (const siteFix of siteFixes) {
    const findingID = siteFix.doctor_finding_id?.trim();
    if (!findingID) continue;
    const current = latest.get(findingID);
    if (!current || isNewerSiteFix(siteFix, current)) latest.set(findingID, siteFix);
  }
  return latest;
}

export function siteFixHasCreatedPR(siteFix: SiteFix) {
  const application = siteFix.application;
  return Boolean(
    application?.github_pr_url?.trim() ||
      application?.github_pr_number != null ||
      application?.pr_created_at,
  );
}

export function activeDoctorFindings<TFinding extends DoctorFindingReference>(findings: TFinding[], siteFixes: SiteFix[]) {
  const handedOffFindingIDs = new Set(siteFixes.map((siteFix) => siteFix.doctor_finding_id?.trim()).filter(Boolean));
  return findings.filter((finding) => !handedOffFindingIDs.has(finding.id));
}

export function recentDoctorFindingLinks<TFinding extends DoctorFindingReference>(
  findings: TFinding[],
  siteFixes: SiteFix[],
): DoctorRecentFindingLink<TFinding>[] {
  const latestByFinding = latestSiteFixByFinding(siteFixes);
  const links: DoctorRecentFindingLink<TFinding>[] = [];
  for (const finding of findings) {
    const siteFix = latestByFinding.get(finding.id);
    if (!siteFix || siteFix.doctor_link_dismissed_at || siteFixHasCreatedPR(siteFix)) continue;
    links.push({ finding, siteFix });
  }
  return links.sort((left, right) => {
    const timeDifference = siteFixCreatedRank(right.siteFix) - siteFixCreatedRank(left.siteFix);
    if (timeDifference !== 0) return timeDifference;
    return right.siteFix.id.localeCompare(left.siteFix.id);
  });
}
