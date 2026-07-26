(() => {
  const hero = document.querySelector(".hero");
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const pageLang = (document.documentElement.lang || "zh-CN").toLowerCase();
  const isEn = pageLang === "en" || pageLang.startsWith("en-");

  const COPY = isEn
    ? {
        heroTitles: ["Self-improve · Orchestration", "Delivery · Orchestration", "Visual review · Orchestration"],
        heroTitleIdle: "Delivery · Orchestration",
        parAsk: "Run sign-in, billing, and notifications in parallel — start now.",
        parReply: "Got it. Parallel workflows for the three requirements are up and running.",
        pmPlaceholder: "Say the next thing to ApprovingPM…",
        idleMeta: "Idle",
        bootingMeta: "Booting",
        readyMeta: "Ready",
        busyMeta: "Running",
        waitingGateMeta: "Waiting on gate",
        doneMeta: "Done",
        capParStart: "IM confirmed — running in parallel…",
        capParBoot: "Confirmed — spinning up parallel runtimes",
        capParRun: "Three requirements running in parallel — research / plan",
        capParImpl: "Implement writing code in parallel — no interference",
        capParTest: "Tests keep running; billing waits at a gate — parallel, not blocked",
        capParShip: "Two MRs submitted; billing still awaiting approval",
        capParGate: "Gate cleared — billing submits its MR",
        capParDone: "IM start → parallel run → closed out",
        approveTitleReject: "Sign-in · UI DEMO",
        approveDescReject: "Review an interactive demo — annotate issues, then reject or approve.",
        approveTitlePass: "Session renew · UI DEMO",
        approveDescPass: "Issues fixed. Review the demo again — approve if it looks good.",
        capApproveOpen: "Open the approval window — reviewing a UI demo",
        capApproveAnnotate: "Annotate the issue: primary button label is wrong",
        approveRejectComment:
          'Reject: primary button should not be "Delete account" — change it back to "Sign in / Continue".',
        capApproveReject: "Reject case — send back with annotation feedback",
        capApproveNext: "Next item: session renew demo — approve if clean",
        approvePassComment: "LGTM — demo flow and copy match. Approved.",
        capApproveJust: "Just Approve",
        capApproveDone: "Annotate reject / demo approve — both paths covered",
        pmAsk:
          "How do we guarantee quality? Research industry harness practices and software engineering experience for this open-source project.",
        pmConfirm: "Confirmed — start the workflow with this plan.",
      }
    : {
        heroTitles: ["自我迭代 · 编排", "需求交付 · 编排", "视觉评审 · 编排"],
        heroTitleIdle: "需求交付 · 编排",
        parAsk: "并行推进登录鉴权、计费结算、消息通知这三个需求，现在启动。",
        parReply: "收到。已按三条需求拉起并行工作流，开始执行。",
        pmPlaceholder: "跟 ApprovingPM 说下一句…",
        idleMeta: "待机",
        bootingMeta: "拉起中",
        readyMeta: "就绪",
        busyMeta: "执行中",
        waitingGateMeta: "等待门禁",
        doneMeta: "完成",
        capParStart: "IM 确认启动，并行推进中…",
        capParBoot: "已确认启动 — 拉起并行执行环境",
        capParRun: "三条需求并行开跑 — research / plan",
        capParImpl: "Implement 并行写代码 — 互不干扰",
        capParTest: "测试继续跑；计费停在门禁 — 并行不互相堵",
        capParShip: "两条已提交 MR；计费仍等批准",
        capParGate: "门禁通过 — 计费补交 MR",
        capParDone: "IM 启动 → 并行执行 → 收口完成",
        approveTitleReject: "登录鉴权 · UI DEMO",
        approveDescReject: "审阅可交互 Demo — 框选问题区域后可拒绝或批准。",
        approveTitlePass: "会话续期 · UI DEMO",
        approveDescPass: "问题已修。再看一遍 Demo，没问题即可批准。",
        capApproveOpen: "打开审批窗口 — 审的是 UI Demo",
        capApproveAnnotate: "框选问题区域：主按钮文案不对",
        approveRejectComment: "拒绝：主按钮不应是「删除账号」，请改回「登录 / 继续」。",
        capApproveReject: "拒绝 Case — 带着框选反馈打回",
        capApproveNext: "下一单：会话续期 Demo — 无问题可批准",
        approvePassComment: "LGTM — Demo 流程与文案一致，批准。",
        capApproveJust: "Just Approve",
        capApproveDone: "框选拒绝 / Demo 批准 — 两种路径都走通",
        pmAsk:
          "有什么办法可以保证质量？调研一下业界的 harness 和软件工程经验，看看怎么保证这个开源项目的质量。",
        pmConfirm: "确认，按这个方案启动工作流。",
      };

  if (hero) {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => hero.classList.add("is-ready"));
    });
  }

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const lanes = ["auth", "billing", "notify"];

  const heroTitles = COPY.heroTitles;
  const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];
  const jitter = (base, spread = 0.35) => Math.round(base * (1 - spread + Math.random() * spread * 2));

  async function runHeroFlow(root, signal) {
    const live = within(root, "[data-hero-live]");
    const title = within(root, "[data-hero-title]");
    const loop = within(root, "[data-hf-loop]");
    const nodeIds = ["research", "clarify", "visual", "gate", "plan"];
    const edgeIds = ["e1", "e2", "e3", "e4"];
    const actions = ["approve", "revise"];

    const setLive = (text) => {
      if (live) live.textContent = text;
    };

    const setNode = (id, state) => {
      const el = within(root, `[data-hf="${id}"]`);
      el?.classList.remove("is-running", "is-done", "is-waiting");
      if (state) el?.classList.add(`is-${state}`);
    };

    const setEdge = (id, state) => {
      const el = within(root, `[data-hf-e="${id}"]`);
      el?.classList.remove("is-active", "is-done");
      if (state) el?.classList.add(`is-${state}`);
    };

    const setAction = (id, on) => {
      within(root, `[data-hf-action="${id}"]`)?.classList.toggle("is-hot", Boolean(on));
    };

    const reset = () => {
      nodeIds.forEach((id) => setNode(id, null));
      edgeIds.forEach((id) => setEdge(id, null));
      actions.forEach((id) => setAction(id, false));
      loop?.classList.remove("is-show", "is-active");
      setLive("● idle");
    };

    const runNode = async (id, ms, waiting = false) => {
      setNode(id, waiting ? "waiting" : "running");
      await sleep(ms);
      if (signal.aborted) return;
      setNode(id, "done");
    };

    while (!signal.aborted) {
      if (title) title.textContent = pick(heroTitles);
      reset();
      await sleep(jitter(260, 0.2));
      if (signal.aborted) break;

      setLive("● running");
      setEdge("e1", "active");
      await runNode("research", jitter(700));
      if (signal.aborted) break;
      setEdge("e1", "done");

      setEdge("e2", "active");
      await runNode("clarify", jitter(700));
      if (signal.aborted) break;
      setEdge("e2", "done");

      setEdge("e3", "active");
      await runNode("visual", jitter(740));
      if (signal.aborted) break;
      setEdge("e3", "done");

      setLive("● waiting");
      setNode("gate", "waiting");
      await sleep(jitter(900, 0.25));
      if (signal.aborted) break;

      if (Math.random() > 0.45) {
        setAction("revise", true);
        loop?.classList.add("is-show", "is-active");
        setLive("● revise");
        await sleep(jitter(850, 0.2));
        if (signal.aborted) break;

        setAction("revise", false);
        loop?.classList.remove("is-active");
        setNode("gate", null);
        setNode("visual", null);
        setEdge("e3", null);

        setEdge("e3", "active");
        await runNode("visual", jitter(640));
        if (signal.aborted) break;
        setEdge("e3", "done");

        setNode("gate", "waiting");
        setLive("● waiting");
        await sleep(jitter(680, 0.25));
        if (signal.aborted) break;
        loop?.classList.remove("is-show");
      }

      setAction("approve", true);
      setLive("● approved");
      await sleep(jitter(420, 0.2));
      if (signal.aborted) break;
      setAction("approve", false);
      setNode("gate", "done");

      setEdge("e4", "active");
      await runNode("plan", jitter(720));
      if (signal.aborted) break;
      setEdge("e4", "done");

      setLive("● done");
      await sleep(jitter(1400, 0.25));
    }
  }

  const heroFlow = document.querySelector("[data-hero-flow]");
  let heroFlowCtrl = null;
  if (heroFlow && !reduceMotion) {
    const startHeroFlow = () => {
      if (heroFlowCtrl) return;
      heroFlowCtrl = new AbortController();
      runHeroFlow(heroFlow, heroFlowCtrl.signal);
    };
    const stopHeroFlow = () => {
      heroFlowCtrl?.abort();
      heroFlowCtrl = null;
    };
    const heroSection = document.querySelector(".hero") || heroFlow;
    if ("IntersectionObserver" in window) {
      const heroIo = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) startHeroFlow();
            else stopHeroFlow();
          }
        },
        { threshold: 0.15 },
      );
      heroIo.observe(heroSection);
    } else {
      startHeroFlow();
    }
  } else if (heroFlow && reduceMotion) {
    const titleEl = within(heroFlow, "[data-hero-title]");
    const liveEl = within(heroFlow, "[data-hero-live]");
    if (titleEl) titleEl.textContent = COPY.heroTitleIdle;
    if (liveEl) liveEl.textContent = "● done";
    ["research", "clarify", "visual", "gate", "plan"].forEach((id) => {
      within(heroFlow, `[data-hf="${id}"]`)?.classList.add("is-done");
    });
    ["e1", "e2", "e3", "e4"].forEach((id) => {
      within(heroFlow, `[data-hf-e="${id}"]`)?.classList.add("is-done");
    });
  }

  function within(root, sel) {
    return root.querySelector(sel);
  }

  function clearNode(el) {
    el?.classList.remove("is-running", "is-done", "is-waiting");
  }

  function setNode(root, id, state) {
    const el = within(root, `[data-node="${id}"]`);
    clearNode(el);
    if (state) el?.classList.add(`is-${state}`);
  }

  function setEdge(root, id, state) {
    const el = within(root, `[data-edge="${id}"]`);
    el?.classList.remove("is-active", "is-done", "is-wait");
    if (state) el?.classList.add(`is-${state}`);
  }

  function setLanePill(root, lane, state, label) {
    const pill = within(root, `[data-lane-pill="${lane}"]`);
    const row = within(root, `[data-lane="${lane}"]`);
    if (pill) {
      pill.className = `demo-lane__pill${state ? ` is-${state}` : ""}`;
      pill.textContent = label;
    }
    row?.classList.remove("is-focus", "is-waiting", "is-done");
    if (state === "running") row?.classList.add("is-focus");
    if (state === "waiting") row?.classList.add("is-waiting");
    if (state === "done") row?.classList.add("is-done");
  }

  function setGates(root, count) {
    const badge = within(root, "[data-gates-badge] b");
    const chip = within(root, "[data-gates-badge]");
    const empty = within(root, "[data-gates-empty]");
    const item = within(root, "[data-gates-item]");
    const gateStat = within(root, "[data-gate-stat]");
    if (badge) badge.textContent = String(count);
    chip?.classList.toggle("is-hot", count > 0);
    empty?.classList.toggle("is-hide", count > 0);
    item?.classList.toggle("is-show", count > 0);
    if (gateStat) gateStat.textContent = `${count} waiting`;
  }

  function setShipStat(root, n) {
    const el = within(root, "[data-run-stat]");
    if (el) el.textContent = `${n} / 3 shipping`;
  }

  function moveCursor(stage, cursor, el) {
    if (!cursor || !el || !stage) return;
    const root = stage.getBoundingClientRect();
    const rect = el.getBoundingClientRect();
    cursor.style.left = `${rect.left - root.left + rect.width * 0.65}px`;
    cursor.style.top = `${rect.top - root.top + rect.height * 0.55}px`;
    cursor.classList.add("is-visible");
  }

  function placeCursorAt(stage, cursor, x, y) {
    if (!cursor || !stage) return;
    cursor.style.left = `${x}px`;
    cursor.style.top = `${y}px`;
    cursor.classList.add("is-visible");
  }

  /** Marquee final box in stage coords: left 52% / top 70% / w 42% / h 18% of .demo-ui */
  function marqueeBoxInStage(stage, ui) {
    const sr = stage.getBoundingClientRect();
    const ur = ui.getBoundingClientRect();
    const left = ur.left - sr.left;
    const top = ur.top - sr.top;
    return {
      x0: left + ur.width * 0.52,
      y0: top + ur.height * 0.7,
      x1: left + ur.width * (0.52 + 0.42),
      y1: top + ur.height * (0.7 + 0.18),
    };
  }

  function animateCursorTo(cursor, x1, y1, duration) {
    return new Promise((resolve) => {
      const x0 = parseFloat(cursor.style.left) || 0;
      const y0 = parseFloat(cursor.style.top) || 0;
      const prev = cursor.style.transition;
      cursor.style.transition = "none";
      let start = null;
      function frame(t) {
        if (start === null) start = t;
        const p = Math.min(1, (t - start) / duration);
        const e = 1 - Math.pow(1 - p, 3);
        cursor.style.left = `${x0 + (x1 - x0) * e}px`;
        cursor.style.top = `${y0 + (y1 - y0) * e}px`;
        if (p < 1) {
          requestAnimationFrame(frame);
        } else {
          cursor.style.transition = prev;
          resolve();
        }
      }
      requestAnimationFrame(frame);
    });
  }

  function hideCursor(cursor) {
    cursor?.classList.remove("is-visible", "is-click");
  }

  async function clickEl(stage, cursor, el) {
    moveCursor(stage, cursor, el);
    await sleep(500);
    cursor?.classList.add("is-click");
    el?.classList.add("is-pressed");
    await sleep(180);
    cursor?.classList.remove("is-click");
    el?.classList.remove("is-pressed");
    hideCursor(cursor);
  }

  async function typeText(el, text, signal, ms = 26) {
    if (!el) return;
    el.textContent = "";
    el.classList.add("is-caret");
    for (let i = 0; i < text.length; i += 1) {
      if (signal.aborted) {
        el.classList.remove("is-caret");
        return;
      }
      el.textContent += text[i];
      await sleep(ms);
    }
    el.classList.remove("is-caret");
  }

  /* Parallel showcase — IM start → runtime containers → lanes */
  const parallelNodes = {
    auth: ["auth-research", "auth-impl", "auth-test", "auth-ship"],
    billing: ["billing-plan", "billing-impl", "billing-gate", "billing-ship"],
    notify: ["notify-research", "notify-impl", "notify-test", "notify-ship"],
  };

  const PAR_ASK = COPY.parAsk;
  const PAR_REPLY = COPY.parReply;
  const TERM_LINES = {
    auth: ["$ boot runtime", "$ mount workspace", "$ agent research …"],
    billing: ["$ boot runtime", "$ mount workspace", "$ agent plan …"],
    notify: ["$ boot runtime", "$ mount workspace", "$ agent research …"],
  };

  function setLiveChip(root, text) {
    const el = within(root, "[data-chip-live]");
    if (el) el.textContent = text;
  }

  function setSbChip(root, n) {
    const el = within(root, "[data-sb-chip] b");
    if (el) el.textContent = String(n);
  }

  function setTermLines(root, id, lines) {
    const card = within(root, `[data-sb="${id}"]`);
    if (!card) return;
    const spans = card.querySelectorAll("[data-sb-line]");
    spans.forEach((el, i) => {
      el.textContent = lines[i] || "";
    });
  }

  function setSandbox(root, id, state, status, meta, lines) {
    const card = within(root, `[data-sb="${id}"]`);
    if (!card) return;
    card.classList.remove("is-creating", "is-running", "is-busy", "is-done");
    if (state) card.classList.add(`is-${state}`);
    const st = within(card, "[data-sb-status]");
    const m = within(card, "[data-sb-meta]");
    if (st) st.textContent = status;
    if (m) m.textContent = meta;
    if (lines) setTermLines(root, id, lines);
  }

  function resetSandboxes(root) {
    for (const id of lanes) {
      setSandbox(root, id, "", "idle", COPY.idleMeta, ["", "", ""]);
    }
    setSbChip(root, 0);
  }

  function hideParMsgs(root) {
    root.querySelectorAll("[data-par-msg]").forEach((el) => {
      el.classList.remove("is-show");
      const out = within(el, "[data-par-typeout]");
      if (out) {
        out.textContent = "";
        out.classList.remove("is-caret");
      }
    });
  }

  function resetParComposer(root) {
    const composer = within(root, "[data-par-composer]");
    if (!composer) return;
    composer.classList.remove("is-typing");
    composer.textContent = "";
    composer.setAttribute("data-placeholder", COPY.pmPlaceholder);
  }

  async function typeParComposer(root, text, signal) {
    const composer = within(root, "[data-par-composer]");
    if (!composer) return;
    composer.classList.add("is-typing");
    composer.removeAttribute("data-placeholder");
    await typeText(composer, text, signal, 28);
    if (signal.aborted) return;
    await sleep(200);
  }

  async function typeParMsg(root, n, signal) {
    const msg = within(root, `[data-par-msg="${n}"]`);
    if (!msg) return;
    const bubble = within(msg, "[data-par-text]");
    const out = within(msg, "[data-par-typeout]");
    const full = bubble?.dataset.parText || "";
    if (out) out.textContent = "";
    msg.classList.add("is-show");
    await typeText(out, full, signal, 20);
  }

  function resetLaunch(root) {
    const sheet = within(root, "[data-launch-sheet]");
    sheet?.classList.add("is-open");
    hideParMsgs(root);
    resetParComposer(root);
  }

  function resetParallel(root) {
    for (const lane of lanes) {
      parallelNodes[lane].forEach((id) => clearNode(within(root, `[data-node="${id}"]`)));
      for (let i = 1; i <= 3; i += 1) setEdge(root, `${lane}-${i}`, "");
      setLanePill(root, lane, "", "queued");
    }
    setGates(root, 0);
    setShipStat(root, 0);
    resetSandboxes(root);
    resetLaunch(root);
    setLiveChip(root, "● idle");
  }

  async function playParallel(root, signal) {
    const caption = within(root, '[data-caption="parallel"]');
    const sheet = within(root, "[data-launch-sheet]");

    while (!signal.aborted) {
      resetParallel(root);
      if (caption) caption.textContent = COPY.capParStart;
      await sleep(300);
      if (signal.aborted) break;

      await typeParComposer(root, PAR_ASK, signal);
      if (signal.aborted) break;

      await typeParMsg(root, 1, signal);
      resetParComposer(root);
      await sleep(280);
      if (signal.aborted) break;

      await typeParMsg(root, 2, signal);
      setLiveChip(root, "● starting");
      if (caption) caption.textContent = COPY.capParBoot;
      await sleep(500);
      if (signal.aborted) break;

      for (const id of lanes) {
        setSandbox(root, id, "creating", "booting", COPY.bootingMeta, [
          "$ init runtime",
          "$ allocate cpu/mem",
          "…",
        ]);
      }
      setSbChip(root, 3);
      await sleep(650);
      if (signal.aborted) break;

      sheet?.classList.remove("is-open");
      setLiveChip(root, "● live run");
      for (const id of lanes) {
        setSandbox(root, id, "running", "ready", COPY.readyMeta, TERM_LINES[id]);
      }
      if (caption) caption.textContent = COPY.capParRun;
      await sleep(400);
      if (signal.aborted) break;

      setNode(root, "auth-research", "running");
      setNode(root, "billing-plan", "running");
      setNode(root, "notify-research", "running");
      setEdge(root, "auth-1", "active");
      setEdge(root, "billing-1", "active");
      setEdge(root, "notify-1", "active");
      setLanePill(root, "auth", "running", "running");
      setLanePill(root, "billing", "running", "running");
      setLanePill(root, "notify", "running", "running");
      setSandbox(root, "auth", "busy", "busy", COPY.busyMeta, ["$ agent research", "writing brief…", "ok"]);
      setSandbox(root, "billing", "busy", "busy", COPY.busyMeta, ["$ agent plan", "draft model…", "ok"]);
      setSandbox(root, "notify", "busy", "busy", COPY.busyMeta, ["$ agent research", "channel pick…", "ok"]);
      await sleep(900);
      if (signal.aborted) break;

      setNode(root, "auth-research", "done");
      setNode(root, "billing-plan", "done");
      setNode(root, "notify-research", "done");
      setEdge(root, "auth-1", "done");
      setEdge(root, "billing-1", "done");
      setEdge(root, "notify-1", "done");
      setNode(root, "auth-impl", "running");
      setNode(root, "billing-impl", "running");
      setNode(root, "notify-impl", "running");
      setEdge(root, "auth-2", "active");
      setEdge(root, "billing-2", "active");
      setEdge(root, "notify-2", "active");
      setSandbox(root, "auth", "busy", "busy", COPY.busyMeta, ["$ implement", "edit middleware…", "tests…"]);
      setSandbox(root, "billing", "busy", "busy", COPY.busyMeta, ["$ implement", "edit ledger…", "tests…"]);
      setSandbox(root, "notify", "busy", "busy", COPY.busyMeta, ["$ implement", "edit webhook…", "tests…"]);
      if (caption) caption.textContent = COPY.capParImpl;
      await sleep(1000);
      if (signal.aborted) break;

      setNode(root, "auth-impl", "done");
      setNode(root, "billing-impl", "done");
      setNode(root, "notify-impl", "done");
      setEdge(root, "auth-2", "done");
      setEdge(root, "billing-2", "done");
      setEdge(root, "notify-2", "done");

      setNode(root, "auth-test", "running");
      setNode(root, "notify-test", "running");
      setNode(root, "billing-gate", "waiting");
      setEdge(root, "auth-3", "active");
      setEdge(root, "notify-3", "active");
      setEdge(root, "billing-3", "wait");
      setLanePill(root, "billing", "waiting", "waiting human");
      setGates(root, 1);
      setSandbox(root, "auth", "busy", "busy", COPY.busyMeta, ["$ e2e", "playwright…", "green"]);
      setSandbox(root, "billing", "running", "paused", COPY.waitingGateMeta, ["$ gate", "waiting human…", ""]);
      setSandbox(root, "notify", "busy", "busy", COPY.busyMeta, ["$ contract test", "retries…", "ok"]);
      if (caption) caption.textContent = COPY.capParTest;
      await sleep(1100);
      if (signal.aborted) break;

      setNode(root, "auth-test", "done");
      setNode(root, "notify-test", "done");
      setEdge(root, "auth-3", "done");
      setEdge(root, "notify-3", "done");
      setNode(root, "auth-ship", "running");
      setNode(root, "notify-ship", "running");
      setShipStat(root, 2);
      setSandbox(root, "auth", "busy", "busy", COPY.busyMeta, ["$ submit MR", "!482", "done"]);
      setSandbox(root, "notify", "busy", "busy", COPY.busyMeta, ["$ submit MR", "!491", "done"]);
      if (caption) caption.textContent = COPY.capParShip;
      await sleep(900);
      if (signal.aborted) break;

      setNode(root, "auth-ship", "done");
      setNode(root, "notify-ship", "done");
      setLanePill(root, "auth", "done", "shipped");
      setLanePill(root, "notify", "done", "shipped");
      setSandbox(root, "auth", "done", "done", COPY.doneMeta, ["$ idle", "", ""]);
      setSandbox(root, "notify", "done", "done", COPY.doneMeta, ["$ idle", "", ""]);
      setNode(root, "billing-gate", "done");
      setEdge(root, "billing-3", "done");
      setGates(root, 0);
      setNode(root, "billing-ship", "running");
      setLanePill(root, "billing", "running", "shipping");
      setSandbox(root, "billing", "busy", "busy", COPY.busyMeta, ["$ submit MR", "push…", "done"]);
      setShipStat(root, 3);
      if (caption) caption.textContent = COPY.capParGate;
      await sleep(700);
      if (signal.aborted) break;

      setNode(root, "billing-ship", "done");
      setLanePill(root, "billing", "done", "shipped");
      setSandbox(root, "billing", "done", "done", COPY.doneMeta, ["$ idle", "", ""]);
      if (caption) caption.textContent = COPY.capParDone;
      await sleep(1500);
    }
  }

  /* Approve showcase — UI demo, marquee annotate, reject then approve */
  async function typeComment(el, text, signal, ms = 22) {
    if (!el) return;
    el.textContent = "";
    el.classList.add("is-typing");
    for (let i = 0; i < text.length; i += 1) {
      if (signal.aborted) {
        el.classList.remove("is-typing");
        return;
      }
      el.textContent += text[i];
      await sleep(ms);
    }
    el.classList.remove("is-typing");
  }

  function setApproveCase(root, which) {
    const rejectUi = within(root, '[data-demo-case="reject"]');
    const passUi = within(root, '[data-demo-case="pass"]');
    const title = within(root, "[data-approve-title]");
    const desc = within(root, "[data-approve-desc]");
    const pill = within(root, "[data-approve-pill]");
    const marquee = within(root, "[data-marquee]");
    const railReject = within(root, '[data-approve-rail="reject"]');
    const railPass = within(root, '[data-approve-rail="pass"]');
    marquee?.classList.remove("is-show");
    if (which === "reject") {
      if (rejectUi) rejectUi.hidden = false;
      if (passUi) passUi.hidden = true;
      if (title) title.textContent = COPY.approveTitleReject;
      if (desc) desc.textContent = COPY.approveDescReject;
      if (pill) {
        pill.textContent = "pending";
        pill.classList.remove("is-rejected", "is-approved");
      }
      railReject?.classList.add("is-active");
      railReject?.classList.remove("is-done");
      railPass?.classList.remove("is-active");
    } else {
      if (rejectUi) rejectUi.hidden = true;
      if (passUi) passUi.hidden = false;
      if (title) title.textContent = COPY.approveTitlePass;
      if (desc) desc.textContent = COPY.approveDescPass;
      if (pill) {
        pill.textContent = "pending";
        pill.classList.remove("is-rejected", "is-approved");
      }
      railReject?.classList.remove("is-active");
      railReject?.classList.add("is-done");
      railPass?.classList.add("is-active");
      railPass?.classList.remove("is-done");
    }
  }

  async function playApprove(root, signal) {
    const caption = within(root, '[data-caption="approve"]');
    const stage = within(root, ".approve-stage");
    const win = within(root, "[data-approve-window]");
    const approveBtn = within(root, "[data-approve-btn]");
    const rejectBtn = within(root, "[data-reject-btn]");
    const cursor = within(root, '[data-cursor="approve"]');
    const comment = within(root, "[data-approve-comment]");
    const marquee = within(root, "[data-marquee]");
    const pill = within(root, "[data-approve-pill]");
    const railPass = within(root, '[data-approve-rail="pass"]');

    while (!signal.aborted) {
      win?.classList.remove("is-open");
      setApproveCase(root, "reject");
      if (comment) comment.textContent = "";
      if (caption) caption.textContent = COPY.capApproveOpen;
      await sleep(350);
      if (signal.aborted) break;

      win?.classList.add("is-open");
      await sleep(reduceMotion ? 300 : 700);
      if (signal.aborted) break;

      if (caption) caption.textContent = COPY.capApproveAnnotate;
      const target = within(root, "[data-annotate-target]");
      const demoUi = target?.closest(".demo-ui") || within(root, '[data-demo-case="reject"]');
      if (target && cursor && stage && demoUi) {
        /* Frame-select path: NW tip at marquee top-left → drag to bottom-right with is-show (no is-click) */
        const box = marqueeBoxInStage(stage, demoUi);
        const prevTransition = cursor.style.transition;
        cursor.style.transition = "none";
        placeCursorAt(stage, cursor, box.x0 - 28, box.y0 - 24);
        void cursor.offsetWidth;
        cursor.style.transition = prevTransition;
        await animateCursorTo(cursor, box.x0, box.y0, 380);
        await sleep(120);
        if (signal.aborted) break;
        marquee?.classList.add("is-show");
        await animateCursorTo(cursor, box.x1, box.y1, 550);
        await sleep(280);
        if (signal.aborted) break;
        hideCursor(cursor);
      } else {
        marquee?.classList.add("is-show");
      }
      await sleep(400);
      if (signal.aborted) break;

      await typeComment(comment, COPY.approveRejectComment, signal);
      if (signal.aborted) break;
      await sleep(250);
      if (signal.aborted) break;

      if (caption) caption.textContent = COPY.capApproveReject;
      await clickEl(stage, cursor, rejectBtn);
      if (signal.aborted) break;
      if (pill) {
        pill.textContent = "rejected";
        pill.classList.add("is-rejected");
      }
      await sleep(700);
      if (signal.aborted) break;

      setApproveCase(root, "pass");
      if (comment) comment.textContent = "";
      if (caption) caption.textContent = COPY.capApproveNext;
      await sleep(550);
      if (signal.aborted) break;

      await typeComment(comment, COPY.approvePassComment, signal);
      if (signal.aborted) break;
      await sleep(250);
      if (signal.aborted) break;

      if (caption) caption.textContent = COPY.capApproveJust;
      await clickEl(stage, cursor, approveBtn);
      if (signal.aborted) break;
      if (pill) {
        pill.textContent = "approved";
        pill.classList.remove("is-rejected");
        pill.classList.add("is-approved");
      }
      railPass?.classList.add("is-done");
      if (caption) caption.textContent = COPY.capApproveDone;
      await sleep(1400);
    }
  }

  /* PM showcase — typewriter IM, no bottom caption */
  const PM_PLACEHOLDER = COPY.pmPlaceholder;

  function scrollPmThread(root, behavior = "smooth") {
    const thread = within(root, "[data-pm-thread]");
    if (!thread) return;
    thread.scrollTo({ top: thread.scrollHeight, behavior });
  }


  async function typeComposer(root, text, signal) {
    const composer = within(root, "[data-pm-composer]");
    if (!composer) return;
    composer.classList.add("is-typing");
    composer.removeAttribute("data-placeholder");
    await typeText(composer, text, signal, 30);
    if (signal.aborted) return;
    await sleep(220);
  }

  function resetComposer(root) {
    const composer = within(root, "[data-pm-composer]");
    if (!composer) return;
    composer.classList.remove("is-typing");
    composer.textContent = "";
    composer.setAttribute("data-placeholder", PM_PLACEHOLDER);
  }

  async function typeMsg(root, n, signal) {
    const msg = within(root, `[data-pm-msg="${n}"]`);
    if (!msg) return;
    const bubble = within(msg, "[data-pm-text]");
    const out = within(msg, "[data-pm-typeout]");
    const extra = within(msg, "[data-pm-extra]");
    const full = bubble?.dataset.pmText || "";
    if (out) out.textContent = "";
    if (extra) extra.hidden = true;
    msg.classList.add("is-show");
    requestAnimationFrame(() => scrollPmThread(root, "auto"));
    await typeText(out, full, signal, 22);
    if (signal.aborted) return;
    if (extra) {
      extra.hidden = false;
      scrollPmThread(root);
    } else {
      scrollPmThread(root);
    }
  }

  function hideAllMsgs(root) {
    root.querySelectorAll("[data-pm-msg]").forEach((el) => {
      el.classList.remove("is-show");
      const out = within(el, "[data-pm-typeout]");
      if (out) {
        out.textContent = "";
        out.classList.remove("is-caret");
      }
      const extra = within(el, "[data-pm-extra]");
      if (extra) extra.hidden = true;
    });
    const thread = within(root, "[data-pm-thread]");
    if (thread) thread.scrollTop = 0;
  }

  function setCounts(root, running, waiting) {
    const r = within(root, "[data-pm-running]");
    const w = within(root, "[data-pm-waiting]");
    if (r) r.textContent = String(running);
    if (w) w.textContent = String(waiting);
  }

  function setStep(root, n, state, label) {
    const li = within(root, `[data-pm-step="${n}"]`);
    const tag = within(root, `[data-pm-step-tag="${n}"]`);
    if (li) {
      li.classList.remove("is-proposed", "is-running", "is-waiting", "is-done");
      if (state) li.classList.add(`is-${state}`);
    }
    if (tag) tag.textContent = label;
  }

  function resetSteps(root) {
    for (let i = 1; i <= 3; i += 1) setStep(root, i, "", "proposed");
    for (let i = 4; i <= 6; i += 1) setStep(root, i, "", "queued");
  }

  async function playPm(root, signal) {
    const ask = COPY.pmAsk;
    const confirm = COPY.pmConfirm;

    while (!signal.aborted) {
      hideAllMsgs(root);
      resetSteps(root);
      setCounts(root, 0, 0);
      resetComposer(root);
      await sleep(350);
      if (signal.aborted) break;

      await typeComposer(root, ask, signal);
      if (signal.aborted) break;

      await typeMsg(root, 1, signal);
      resetComposer(root);
      await sleep(280);
      if (signal.aborted) break;

      await typeMsg(root, 2, signal);
      await sleep(400);
      if (signal.aborted) break;

      await typeMsg(root, 3, signal);
      setStep(root, 1, "proposed", "proposed");
      setStep(root, 2, "proposed", "proposed");
      setStep(root, 3, "proposed", "proposed");
      scrollPmThread(root);
      await sleep(500);
      if (signal.aborted) break;

      await typeMsg(root, 4, signal);
      setCounts(root, 0, 1);
      await sleep(400);
      if (signal.aborted) break;

      await typeComposer(root, confirm, signal);
      if (signal.aborted) break;

      await typeMsg(root, 5, signal);
      resetComposer(root);
      setCounts(root, 0, 0);
      await sleep(280);
      if (signal.aborted) break;

      await typeMsg(root, 6, signal);
      setStep(root, 4, "running", "running");
      setStep(root, 5, "running", "running");
      setStep(root, 6, "queued", "queued");
      setCounts(root, 2, 0);
      scrollPmThread(root);
      await sleep(700);
      if (signal.aborted) break;

      setStep(root, 4, "done", "done");
      setStep(root, 5, "running", "60%");
      setStep(root, 6, "waiting", "pending");
      setCounts(root, 1, 1);
      await typeMsg(root, 7, signal);
      await sleep(1500);
    }
  }

  const players = {
    parallel: playParallel,
    approve: playApprove,
    pm: playPm,
  };

  const controllers = new Map();

  function startShowcase(el) {
    const key = el.dataset.showcase;
    const play = players[key];
    if (!play || controllers.has(key)) return;

    if (reduceMotion) {
      if (key === "parallel") {
        resetParallel(el);
        within(el, "[data-launch-sheet]")?.classList.remove("is-open");
        Object.values(parallelNodes).flat().forEach((id) => setNode(el, id, "done"));
        for (const lane of lanes) {
          for (let i = 1; i <= 3; i += 1) setEdge(el, `${lane}-${i}`, "done");
          setLanePill(el, lane, "done", "shipped");
          setSandbox(el, lane, "done", "done", COPY.doneMeta, ["$ idle", "", ""]);
        }
        setShipStat(el, 3);
        setSbChip(el, 3);
        setLiveChip(el, "● live run");
      } else if (key === "approve") {
        within(el, "[data-approve-window]")?.classList.add("is-open");
        setApproveCase(el, "pass");
        const c = within(el, "[data-approve-comment]");
        if (c) c.textContent = COPY.approvePassComment;
        const pill = within(el, "[data-approve-pill]");
        if (pill) {
          pill.textContent = "approved";
          pill.classList.add("is-approved");
        }
      } else if (key === "pm") {
        el.querySelectorAll("[data-pm-msg]").forEach((m) => {
          m.classList.add("is-show");
          const bubble = m.querySelector("[data-pm-text]");
          const out = m.querySelector("[data-pm-typeout]");
          const extra = m.querySelector("[data-pm-extra]");
          if (out && bubble) out.textContent = bubble.dataset.pmText || "";
          if (extra) extra.hidden = false;
        });
        for (let i = 1; i <= 3; i += 1) setStep(el, i, "proposed", "proposed");
        for (let i = 4; i <= 6; i += 1) setStep(el, i, "done", "done");
        setCounts(el, 0, 0);
        resetComposer(el);
        scrollPmThread(el, "auto");
      }
      return;
    }

    const ac = new AbortController();
    controllers.set(key, ac);
    play(el, ac.signal);
  }

  function stopShowcase(el) {
    const key = el.dataset.showcase;
    const ac = controllers.get(key);
    if (ac) {
      ac.abort();
      controllers.delete(key);
    }
  }

  const sections = document.querySelectorAll("[data-showcase]");
  if (!("IntersectionObserver" in window)) {
    sections.forEach((el) => startShowcase(el));
    return;
  }

  const io = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting && entry.intersectionRatio >= 0.35) {
          startShowcase(entry.target);
        } else {
          stopShowcase(entry.target);
        }
      }
    },
    { threshold: [0, 0.35, 0.6] },
  );

  sections.forEach((el) => io.observe(el));
})();
