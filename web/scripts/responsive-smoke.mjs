#!/usr/bin/env node

import { chromium } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { assessResponsiveMetrics, corePagePaths, VIEWPORTS } from "../app/lib/responsive-smoke-config.mjs";

const baseURL = process.env.CITELOOP_RESPONSIVE_BASE_URL || "http://localhost:3000";
const projectId = process.env.CITELOOP_RESPONSIVE_PROJECT_ID;
const storageState = process.env.CITELOOP_RESPONSIVE_STORAGE_STATE;
const outputDir = process.env.CITELOOP_RESPONSIVE_OUTPUT_DIR || path.join("test-results", "responsive-smoke");
const headless = process.env.PLAYWRIGHT_HEADLESS !== "0";
const channel = process.env.PLAYWRIGHT_CHANNEL || undefined;

if (!projectId) {
  console.error("CITELOOP_RESPONSIVE_PROJECT_ID is required for project page smoke checks.");
  process.exit(2);
}

await mkdir(outputDir, { recursive: true });

const browser = await chromium.launch({ channel, headless });
const failures = [];
const reports = [];

try {
  for (const viewport of VIEWPORTS) {
    const context = await browser.newContext({
      storageState: storageState || undefined,
      viewport: { width: viewport.width, height: viewport.height },
    });
    const page = await context.newPage();

    for (const target of corePagePaths(projectId)) {
      const url = new URL(target.path, baseURL).toString();
      await page.goto(url, { waitUntil: "networkidle", timeout: 45000 });
      const landedPath = new URL(page.url()).pathname;

      const fileBase = `${viewport.name}-${target.label.toLowerCase().replaceAll(" ", "-")}`;
      const screenshotPath = path.join(outputDir, `${fileBase}.png`);
      await page.screenshot({ fullPage: true, path: screenshotPath });

      const metrics = await page.evaluate(({ label, viewportName, targetPath, landedPath }) => {
        const viewportWidth = document.documentElement.clientWidth;
        const documentWidth = Math.max(document.documentElement.scrollWidth, document.body?.scrollWidth || 0);
        const visible = (el) => {
          const rect = el.getBoundingClientRect();
          const style = window.getComputedStyle(el);
          return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
        };
        const textFor = (el) => (el.getAttribute("aria-label") || el.textContent || el.getAttribute("href") || el.tagName).trim().replace(/\s+/g, " ").slice(0, 80);
        const rectFor = (el) => {
          const rect = el.getBoundingClientRect();
          return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height };
        };
        const controls = Array.from(document.querySelectorAll("button,a,[role='button'],input,textarea,select"))
          .filter(visible)
          .map((el) => ({ el, label: textFor(el), rect: rectFor(el) }));
        const overlappingControls = [];
        for (let i = 0; i < controls.length; i += 1) {
          for (let j = i + 1; j < controls.length; j += 1) {
            const a = controls[i];
            const b = controls[j];
            if (a.el.contains(b.el) || b.el.contains(a.el)) continue;
            const horizontal = Math.min(a.rect.right, b.rect.right) - Math.max(a.rect.left, b.rect.left);
            const vertical = Math.min(a.rect.bottom, b.rect.bottom) - Math.max(a.rect.top, b.rect.top);
            if (horizontal > 4 && vertical > 4) {
              overlappingControls.push({ a: a.label, b: b.label });
            }
          }
        }
        const primaryElements = Array.from(document.querySelectorAll("main,section,article,h1,h2,h3,pre,textarea,button,a,input,select"))
          .filter(visible)
          .map((el) => ({ el, label: textFor(el), rect: rectFor(el) }));
        const protrudingElements = primaryElements
          .filter((item) => item.rect.left < -1 || item.rect.right > viewportWidth + 1)
          .filter((item) => {
            let current = item.el.parentElement;
            while (current && current !== document.body) {
              const style = window.getComputedStyle(current);
              if (style.overflowX === "auto" || style.overflowX === "scroll") return false;
              current = current.parentElement;
            }
            return true;
          })
          .slice(0, 8)
          .map((item) => ({ label: item.label, left: Math.round(item.rect.left), right: Math.round(item.rect.right) }));
        const disabledControlsWithoutExplanation = Array.from(document.querySelectorAll("button:disabled,[aria-disabled='true']"))
          .filter(visible)
          .filter((el) => {
            if (el.getAttribute("title") || el.getAttribute("aria-describedby")) return false;
            const context = (el.closest("article,section,form,main")?.textContent || "").toLowerCase();
            return !/(required|missing|disabled|blocked|unavailable|connect|enter|select|qa|safe mode|permission|setup|loading)/.test(context);
          })
          .map(textFor)
          .slice(0, 8);
        return {
          page: label,
          viewport: viewportName,
          targetPath,
          landedPath,
          documentWidth,
          viewportWidth,
          protrudingElements,
          overlappingControls: overlappingControls.slice(0, 8),
          disabledControlsWithoutExplanation,
        };
      }, { label: target.label, viewportName: viewport.name, targetPath: target.path, landedPath });

      const pageFailures = assessResponsiveMetrics(metrics);
      failures.push(...pageFailures);
      reports.push({ ...metrics, screenshotPath, failures: pageFailures });
      console.log(`${pageFailures.length ? "FAIL" : "OK"} ${viewport.name} ${target.label} ${target.path}`);
    }

    await context.close();
  }
} finally {
  await browser.close();
}

const reportPath = path.join(outputDir, "report.json");
await writeFile(reportPath, JSON.stringify({ baseURL, projectId, reports, failures }, null, 2));

if (failures.length > 0) {
  console.error(`\nResponsive smoke found ${failures.length} issue(s):`);
  for (const failure of failures) console.error(`- ${failure}`);
  console.error(`\nReport: ${reportPath}`);
  process.exit(1);
}

console.log(`\nResponsive smoke passed. Report: ${reportPath}`);
