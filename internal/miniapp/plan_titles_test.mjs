import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const titleSource = source.slice(source.indexOf("function getPlanCardTitle("), source.indexOf("function getPlanDetails("));
const context = vm.createContext({ state: { locale: "ru" } });
vm.runInContext(titleSource, context);

test("plan cards use localized names and duration when a name is empty", () => {
	const plan = { months: 1, titleRu: "1 месяц", titleEn: "1 month" };
	context.plan = plan;
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'ru')", context), "1 месяц");
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'en')", context), "1 month");
	plan.name = "Месяц — лайт";
	plan.nameEn = "Month Light";
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'ru')", context), "Месяц — лайт");
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'en')", context), "Month Light");
	plan.nameEn = "";
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'en')", context), "1 month");
	delete plan.nameEn;
	plan.titleEn = "Month Light";
	assert.equal(vm.runInContext("getPlanBaseTitle(plan, 'en')", context), "Month Light");
});
