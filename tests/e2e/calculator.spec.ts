import { expect, test } from "@playwright/test";

test("calculator adds non-negative integers without console errors", async ({ page }, testInfo) => {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/calculator");
  await page.getByLabel("Expression").fill("2 + 3");
  await page.getByRole("button", { name: "Calculate" }).click();

  await expect(page.getByRole("status")).toHaveText("5");
  await page.screenshot({ path: testInfo.outputPath("calculator.png"), fullPage: true });
  expect(consoleErrors).toEqual([]);
  expect(pageErrors).toEqual([]);
});
