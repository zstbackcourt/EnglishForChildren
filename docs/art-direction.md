# 场景来源与美术记录

首课彩色背景：`web/assets/scenes/lesson-001.png`。来源：第一册 PDF 第 5 页右侧连环画。工具：内置 imagegen。保留车厢、先生归还女士手提包的情节。已查看生成图，人物、手提包和车厢一致，图中无课文文字，供前端叠加对话。

生成提示词：

> Use case: illustration-story. Create a wide landscape background illustration for an elementary-school English learning RPG, based on the train carriage situation depicted in the right-hand comic panels of the provided Lesson 1 textbook page. Treat the page as reference only. Warm, beautifully crafted hand-painted children's storybook game art, soft gouache textures, forest green and warm amber train interior with large windows showing sunny countryside. A kind dark-haired man on the right returns a small brown handbag to a friendly blonde woman on the left. Preserve the key handbag-returning interaction and train setting from the source panels. Full-bleed single scene, landscape 16:9 composition, characters in middle distance, plenty of clean lower foreground for a UI dialogue box to be overlaid later. Gentle warm sunlight, cozy upholstered seats, polished wood, inviting and calm. No writing, no letters, no speech bubbles, no UI, no borders, no watermark.

Lesson 002–144 均已使用对应教材页作为内容参考、首课作为画风参考逐课重绘。每课独立 PNG，提示词位于 `docs/art-prompts/lesson-NNN.txt`，页面映射见 `content/book1.json`。故事课保留核心情节，练习课用多格画面保留物品、动作和比较关系；不复用首课场景。全部插图完整展示，对话框置于图片下方，并提供放大插画与查看教材入口。详细实施与验收记录见 `lesson-art-upgrade.md`。
