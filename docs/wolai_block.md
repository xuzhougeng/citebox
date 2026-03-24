# Wolai Block Types

Wolai (我来) is a Chinese cloud‑based note‑taking and collaboration platform that follows a block‑based model similar to Notion. While the original Wolai documentation could not be accessed due to network restrictions, the core block types correspond closely to the block definitions used in Notion. Therefore this document synthesizes information about Wolai block types by combining the list of block names from a Wolai‑to‑Notion converter’s source code with detailed explanations from publicly available documentation (primarily from Notion’s API and help center). The resulting descriptions apply broadly to block‑based editors and should provide readers with an understanding of how each block works in Wolai.

## Block type overview

The Python tool **wolai2notion**, which converts Wolai blocks to Notion, exposes a list of block types supported in Wolai. Its `BlockType` class defines the following types[\[1\]](https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md#:~:text=%60%60%60python%20,%E5%BC%95%E7%94%A8):

| Block type                   | Meaning (from code)                 | Notes                                                                       |
| ---------------------------- | ----------------------------------- | --------------------------------------------------------------------------- |
| `HEADING`                    | **标题** – heading; may be toggleable | Supports multiple levels (H1–H9) and optionally collapsible.                |
| `ENUM_LIST`                  | **有序列表** – numbered list            | Each list item has a number; lists can nest.                                |
| `BULL_LIST`                  | **无序列表** – bulleted list            | Uses bullet points; lists can nest.                                         |
| `TOGGLE_LIST`                | **折叠列表** – toggle (collapsed) list  | Each item can be collapsed/expanded; nested content appears when expanded.  |
| `CODE`                       | **代码块** – code block                | Displays code with syntax highlighting.                                     |
| `IMAGE`                      | **图片块** – image                     | Contains an image file plus optional caption.                               |
| `VIDEO`                      | **视频块** – video                     | Embeds or uploads a video file.                                             |
| `QUOTE`                      | **引用块** – quote                     | Displays quoted text with special formatting.                               |
| `TEXT`                       | **文本块** – plain text                | Basic paragraph or paragraph with rich‑text formatting.                     |
| `BOOKMARK`                   | **书签块** – bookmark                  | Displays metadata of a URL.                                                 |
| `DIVIDER`                    | **分割线** – divider                   | Inserts a horizontal line to separate sections.                             |
| `TABLE`                      | **表格块** – table                     | Simple table with rows and columns.                                         |
| `CALLOUT`                    | **标注框** – callout                   | A highlighted box with an icon and colored background to emphasize content. |
| `BLOCK_EQUATION`             | **公式块** – math equation             | Displays a LaTeX/KaTeX block‑level equation.                                |
| `REFERENCE`                  | **引用块** – reference                 | Embeds or links another block or page.                                      |
| `BOLD` (content type)        | bold text                           | Marks text as bold within a block.                                          |
| `INLINE_CODE` (content type) | inline code                         | Formats short pieces of code within text.                                   |
| `TEXT` (content type)        | regular text                        | The default text style within a block.                                      |

Wolai groups these blocks into **basic** (text, headings, lists, code snippets, math formulas etc.), **advanced** (toggle lists, tables, mind maps, meeting templates, mermaid charts etc.), and **media/attachment** types (pictures, audio/video, website bookmarks and other third‑party embeds). This classification is described in a Medium article comparing Wolai and Yuque[\[2\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=Wolai%20provides%20three%20main%20categories,of%20inserted%20block%20content).

## Detailed block descriptions

### Heading block (`HEADING`/`标题`)

A heading block defines the structure of a document. Notion supports three heading levels in its API; the concept extends in Wolai to nine levels (H1–H9). According to Notionpresso’s documentation, heading blocks include the block type (`heading_1`, `heading_2` or `heading_3`), rich‑text content, a color and an `is_toggleable` property that indicates whether the heading can collapse child content[\[3\]](https://notionpresso.com/en/docs/block-types/notion-block-heading#:~:text=Heading%20Block). Heading blocks are used to create sections, provide hierarchy, and (when toggleable) act like collapsed headings.

**Key properties**[\[3\]](https://notionpresso.com/en/docs/block-types/notion-block-heading#:~:text=Heading%20Block):

  - `type`: identifies the level (`heading_1`, `heading_2` or `heading_3`). Wolai likely supports H1–H9.
  - `rich_text`: array of text objects containing the heading text and formatting annotations.
  - `color`: text color (e.g., default, red, yellow etc.). Wolai may allow colored headings.
  - `is_toggleable`: whether the heading functions as a toggle; collapsed headings hide their child blocks until expanded.

### Numbered list block (`ENUM_LIST`/`有序列表`)

Numbered list blocks create ordered lists. Notion’s API describes a numbered list item block (`numbered_list_item`) containing rich text, a color, an optional `list_start_index` and `list_format` (`numbers`, `letters` or `roman`)[\[4\]](https://developers.notion.com/reference/block#:~:text=Numbered%20list%20item). Nested numbered lists can be created by adding child blocks. In Wolai, the `/` command likely offers a numbered list option; each item automatically updates its number when items are added or removed.

**Key properties**[\[4\]](https://developers.notion.com/reference/block#:~:text=Numbered%20list%20item):

  - `rich_text`: array of text objects containing the list item content.
  - `color`: text color.
  - `list_start_index` (optional): starting number if the list doesn’t begin at 1.
  - `list_format` (optional): numbering style (numbers, letters or Roman numerals).
  - `children`: nested blocks representing nested lists.

### Bulleted list block (`BULL_LIST`/`无序列表`)

Bulleted list blocks create unordered lists. The Notionpresso guide explains that a bulleted list item block is used to create lists where each item starts with a bullet point. The API representation includes `rich_text` for the item’s content and a `color` property. Bulleted lists can be nested; consecutive bulleted list items are grouped automatically into a single list and nested items change their bullet style.

### Toggle list block (`TOGGLE_LIST`/`折叠列表`)

A toggle list block acts like a collapsible list. Each list item displays a caret icon; clicking it reveals or hides the nested content. Notion’s API supports toggle blocks (sometimes called “toggle list” or “toggle heading”). Each toggle contains rich text, a color, and can have child blocks. Toggle lists are useful for reducing clutter and revealing details on demand. In Wolai, the `折叠列表` block type corresponds to this functionality; users can create a toggle list using `/toggle` or by converting other blocks.

### Code block (`CODE`/`代码块`)

Code blocks display code in a monospaced font with syntax highlighting. The Notion help center notes that you can create a code block by clicking the `+` button and choosing **Code**, or typing `/code`, which inserts a code block with its own language selector[\[5\]](https://www.notion.com/help/code-blocks#:~:text=Add%20a%20code%20block). Users can select the programming language for syntax highlighting, wrap long lines, copy code easily, and add captions[\[6\]](https://www.notion.com/help/code-blocks#:~:text=On%20any%20Notion%20page%2C%20you,content%20and%20formatted%20by%20language). Wolai’s code block likely offers similar features such as language selection, line numbers and an optional caption.

### Image block (`IMAGE`/`图片块`)

Image blocks display an image file with an optional caption. According to the Notion render docs, an image block contains an image file (supported formats include bmp, gif, heic, jpeg/jpg, png, svg, tif/tiff) and may include a caption[\[7\]](https://notion-render-docs.vercel.app/blocks/image#:~:text=Image). It doesn’t require special client rendering. In Wolai, users can upload or embed images via the `/image` command or by drag‑and‑drop. Images can be resized and aligned within columns.

### Video block (`VIDEO`/`视频块`)

Video blocks embed or upload videos. The Notion API specifies that a video block contains a file object with the video URL or file data[\[8\]](https://developers.notion.com/reference/block#:~:text=Video). Supported video types include many common formats (e.g., `.mp4`, `.avi`, `.mov`) and YouTube links with `embed` or `watch` keywords[\[9\]](https://developers.notion.com/reference/block#:~:text=Supported%20video%20types). Vimeo links are not supported via the video block and must be embedded using an embed block[\[10\]](https://developers.notion.com/reference/block#:~:text=,id). In Wolai, users can add videos by uploading files or pasting a link from supported video services; membership may be required for uploading large videos.

### Quote block (`QUOTE`/`引用块`)

Quote blocks display quoted text with distinctive formatting. The Notionpresso docs state that a Quote block displays text quoted from another source and includes `rich_text` content and a `color` property[\[11\]](https://notionpresso.com/en/docs/block-types/quote#:~:text=Quote%20Block). Quote blocks often have a vertical bar or indented style and may use italic fonts[\[12\]](https://notionpresso.com/en/docs/block-types/quote#:~:text=%2A%20%60.notion,the%20content%20of%20the%20quote). In Wolai, you can create a quote block with `/quote` or by starting a line with `>` (Markdown syntax).

### Text block (`TEXT`/`文本块`)

A text (paragraph) block contains regular text with optional formatting. The Notion API describes paragraph blocks as containing `rich_text` arrays and a `color` property[\[13\]](https://developers.notion.com/reference/block#:~:text=Paragraph). Text blocks can have nested child blocks (e.g., lists, quotes). They support rich‑text formatting: bold, italic, underline, strikethrough, code, hyperlinks, and color. In Wolai, text blocks are the default block type; hitting `Enter` creates a new text block.

### Bookmark block (`BOOKMARK`/`书签块`)

Bookmark blocks display metadata for a specified URL. According to the Notion render docs, a bookmark block shows the metadata (title, description and cover image) of the provided URL[\[14\]](https://notion-render-docs.vercel.app/blocks/bookmark#:~:text=Bookmark). It has the parameters `url` (the target link) and `caption` (optional text below the bookmark)[\[14\]](https://notion-render-docs.vercel.app/blocks/bookmark#:~:text=Bookmark). In Wolai, entering `/bookmark` or pasting a URL can create a bookmark; the platform fetches the website’s metadata to display a preview.

### Divider block (`DIVIDER`/`分割线`)

Divider blocks are simple horizontal rules used to separate content. The Notion render docs describe a divider block as a simple block that splits content on a page[\[15\]](https://notion-render-docs.vercel.app/blocks/divider#:~:text=Divider). It is rendered as an `<hr>` element. In Wolai, typing `---` or `/divider` inserts a divider line.

### Table block (`TABLE`/`表格块`)

Table blocks represent simple tables (not databases) with rows and columns. The Notion render docs state that a table block includes parameters such as width and whether it has column or row headers[\[16\]](https://notion-render-docs.vercel.app/blocks/table#:~:text=Table). In Wolai, table blocks allow editing cell content, merging cells, and formatting, but they are distinct from database tables (data tables). Wolai also supports more advanced data tables that behave like spreadsheets (these may require membership)[\[2\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=Wolai%20provides%20three%20main%20categories,of%20inserted%20block%20content).

### Callout block (`CALLOUT`/`标注框`)

Callout blocks emphasize important information with an icon and colored background. The Notionpresso docs note that a callout block has `rich_text` content, an `icon` (emoji or image) and a `color` property for the background[\[17\]](https://notionpresso.com/en/docs/block-types/callout#:~:text=Callout%20Block). Callout blocks can contain other blocks and are useful for highlighting warnings, tips or summaries[\[18\]](https://notionpresso.com/en/docs/block-types/callout#:~:text=React%20Component). Wolai’s callout block likely supports similar features; users may type `/callout` to insert one.

### Math equation block (`BLOCK_EQUATION`/`公式块`)

A math equation block displays mathematical expressions using LaTeX/KaTeX. The Notion help center explains that you can add a block equation by clicking the `+` button and choosing **Block equation** or typing `/math`[\[19\]](https://www.notion.com/help/math-equations#:~:text=Notion%20uses%20the%C2%A0KaTeX%C2%A0library%C2%A0to%20render%20math,supports%20a%20large%20subset%20of%C2%A0LaTeX%C2%A0functions). Notion uses the KaTeX library to render equations[\[20\]](https://www.notion.com/help/math-equations#:~:text=Notion%20uses%20the%C2%A0KaTeX%C2%A0library%C2%A0to%20render%20math,supports%20a%20large%20subset%20of%C2%A0LaTeX%C2%A0functions). For inline equations, you can surround text with double dollar signs or use the `ctrl/cmd + shift + E` shortcut[\[21\]](https://www.notion.com/help/math-equations#:~:text=Just%20like%20you%20can%20format,equation%2C%20like%20this%20quadratic%20formula). Wolai’s equation block likely behaves the same way and may also support inline formula insertion through a style menu. Wolai appears to support directly pasting LaTeX formulas[\[22\]](https://zhuanlan.zhihu.com/p/19777422849#:~:text=%E7%89%B9%E6%80%A7%2F%E8%BD%AF%E4%BB%B6%20Notion%20Wolai%20Obsidian%20%E5%85%AC%E5%BC%8F%E6%94%AF%E6%8C%81,%E9%9C%80%E8%A6%81%E8%87%AA%E5%B7%B1%E9%85%8D%E7%BD%AE%20%E5%9B%BE%E7%89%87%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E9%9C%80%E8%A6%81%E5%9B%BE%E5%BA%8A), which offers convenience over Notion’s manual formatting.

### Reference block (`REFERENCE`/`引用块`)

Reference blocks allow reusing content elsewhere by embedding or linking to an existing block or page. In Notion, you can link to a specific block by copying its link and pasting it elsewhere; clicking the link scrolls to the block[\[23\]](https://www.notion.vip/insights/link-to-a-specific-block-within-a-notion-page#:~:text=Link%20to%20a%20Specific%20Block,Within%20a%20Notion%20Page). Wolai extends this concept with **行内块引用** (inline block reference) and **嵌入块引用** (embedded block reference), which let you reference a block either inline within text or as a separate block. According to Wolai’s help materials (not directly accessible), references enable bidirectional linking: editing the original block updates all references, and each reference can navigate back to the original. To create a reference, users typically input `[[` followed by a search term or use a right‑sidebar drag‑and‑drop. References support nested content and can be used to tag pages or build indexes.

## Block content types

Wolai’s `BlockContentType` defines the types of content that can appear inside blocks[\[24\]](https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md#:~:text=class%20BlockContentType%3A%20BOLD%20%3D%20%27bold%27,%E6%99%AE%E9%80%9A%E6%96%87%E6%9C%AC):

| Content type             | Description                                                                                                                                              |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `BOLD` (**加粗文本**)        | Marks text as bold for emphasis. It is applied as an annotation within a rich‑text object.                                                               |
| `INLINE_CODE` (**行内代码**) | Formats short code snippets within text (monospaced font). In Notion, this can be created with backticks or via the `/` menu.                            |
| `TEXT` (**普通文本**)        | Represents unstyled text. Rich‑text objects may contain multiple annotations (bold, italic, underline, strikethrough, code, color) alongside plain text. |

## Notes on advanced blocks and membership features

Wolai also offers more advanced blocks beyond those listed in the converter’s `BlockType` class. The Medium article notes that advanced blocks include collapsed headings, collapsed lists, data tables, mind maps, meeting templates, and Mermaid diagrams[\[2\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=Wolai%20provides%20three%20main%20categories,of%20inserted%20block%20content). Some of these advanced blocks (e.g., mind maps and Mermaid charts) may require a paid membership, while basic blocks are available for free. Media blocks such as images, audio, video and bookmarks are considered attachments. Wolai supports seamless embedding of third‑party services like Bilibili videos, Tencent videos and NetEase Cloud Music[\[25\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=,Video%2C%20NetEase%20Cloud%20Music%2C%20etc).

## Creating and manipulating blocks in Wolai

Although Wolai’s official documentation was unavailable, its block editing experience parallels Notion’s. Blocks can be added by typing `/` followed by a block name or by using quick commands (e.g., `/code`, `/math`, `/callout`). Blocks can be moved, nested or converted into other types via drag‑and‑drop or the block menu. Reference blocks enable bidirectional linking between blocks and pages, reducing duplication. Additional formatting features (bold, colors, alignment) are available within text blocks and headings. Wolai appears to allow more heading levels (H1–H9) and supports direct pasting of LaTeX formulas[\[22\]](https://zhuanlan.zhihu.com/p/19777422849#:~:text=%E7%89%B9%E6%80%A7%2F%E8%BD%AF%E4%BB%B6%20Notion%20Wolai%20Obsidian%20%E5%85%AC%E5%BC%8F%E6%94%AF%E6%8C%81,%E9%9C%80%E8%A6%81%E8%87%AA%E5%B7%B1%E9%85%8D%E7%BD%AE%20%E5%9B%BE%E7%89%87%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E9%9C%80%E8%A6%81%E5%9B%BE%E5%BA%8A), offering enhanced flexibility for academic users.

## Conclusion

Because Wolai builds upon the block‑based paradigm popularized by Notion, understanding Notion’s block specifications provides insight into how Wolai’s blocks behave. The `wolai2notion` converter’s enumeration of block types[\[1\]](https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md#:~:text=%60%60%60python%20,%E5%BC%95%E7%94%A8) shows that Wolai supports a comprehensive set of text, list, media and structural blocks. Public documentation from Notion, Notionpresso and other sources offers detailed descriptions and usage guidelines for these block types[\[3\]](https://notionpresso.com/en/docs/block-types/notion-block-heading#:~:text=Heading%20Block)[\[17\]](https://notionpresso.com/en/docs/block-types/callout#:~:text=Callout%20Block)[\[7\]](https://notion-render-docs.vercel.app/blocks/image#:~:text=Image)[\[14\]](https://notion-render-docs.vercel.app/blocks/bookmark#:~:text=Bookmark)[\[15\]](https://notion-render-docs.vercel.app/blocks/divider#:~:text=Divider)[\[19\]](https://www.notion.com/help/math-equations#:~:text=Notion%20uses%20the%C2%A0KaTeX%C2%A0library%C2%A0to%20render%20math,supports%20a%20large%20subset%20of%C2%A0LaTeX%C2%A0functions). Users familiar with Notion should feel comfortable using Wolai’s blocks, while appreciating Wolai’s localized enhancements such as nine heading levels, direct LaTeX support and integration with Chinese media services.

[\[1\]](https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md#:~:text=%60%60%60python%20,%E5%BC%95%E7%94%A8) [\[24\]](https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md#:~:text=class%20BlockContentType%3A%20BOLD%20%3D%20%27bold%27,%E6%99%AE%E9%80%9A%E6%96%87%E6%9C%AC) raw.githubusercontent.com

<https://raw.githubusercontent.com/AruNi-01/wolai2notion/master/README.md>

[\[2\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=Wolai%20provides%20three%20main%20categories,of%20inserted%20block%20content) [\[25\]](https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106#:~:text=,Video%2C%20NetEase%20Cloud%20Music%2C%20etc) In terms of directory structure, wolai manages all files or pages in a tree | by Zannaty Khatun | Medium

<https://medium.com/@zannatykhatun20/in-terms-of-directory-structure-wolai-manages-all-files-or-pages-in-a-tree-e6a43ec27106>

[\[3\]](https://notionpresso.com/en/docs/block-types/notion-block-heading#:~:text=Heading%20Block) Heading Block | Notionpresso Docs

<https://notionpresso.com/en/docs/block-types/notion-block-heading>

[\[4\]](https://developers.notion.com/reference/block#:~:text=Numbered%20list%20item) [\[8\]](https://developers.notion.com/reference/block#:~:text=Video) [\[9\]](https://developers.notion.com/reference/block#:~:text=Supported%20video%20types) [\[10\]](https://developers.notion.com/reference/block#:~:text=,id) [\[13\]](https://developers.notion.com/reference/block#:~:text=Paragraph) Block - Notion Docs

<https://developers.notion.com/reference/block>

[\[5\]](https://www.notion.com/help/code-blocks#:~:text=Add%20a%20code%20block) [\[6\]](https://www.notion.com/help/code-blocks#:~:text=On%20any%20Notion%20page%2C%20you,content%20and%20formatted%20by%20language) Code blocks – Notion Help Center

<https://www.notion.com/help/code-blocks>

[\[7\]](https://notion-render-docs.vercel.app/blocks/image#:~:text=Image) notion-render-docs.vercel.app

<https://notion-render-docs.vercel.app/blocks/image>

[\[11\]](https://notionpresso.com/en/docs/block-types/quote#:~:text=Quote%20Block) [\[12\]](https://notionpresso.com/en/docs/block-types/quote#:~:text=%2A%20%60.notion,the%20content%20of%20the%20quote) Quote Block | Notionpresso Docs

<https://notionpresso.com/en/docs/block-types/quote>

[\[14\]](https://notion-render-docs.vercel.app/blocks/bookmark#:~:text=Bookmark) notion-render-docs.vercel.app

<https://notion-render-docs.vercel.app/blocks/bookmark>

[\[15\]](https://notion-render-docs.vercel.app/blocks/divider#:~:text=Divider) notion-render-docs.vercel.app

<https://notion-render-docs.vercel.app/blocks/divider>

[\[16\]](https://notion-render-docs.vercel.app/blocks/table#:~:text=Table) notion-render-docs.vercel.app

<https://notion-render-docs.vercel.app/blocks/table>

[\[17\]](https://notionpresso.com/en/docs/block-types/callout#:~:text=Callout%20Block) [\[18\]](https://notionpresso.com/en/docs/block-types/callout#:~:text=React%20Component) Callout Block | Notionpresso Docs

<https://notionpresso.com/en/docs/block-types/callout>

[\[19\]](https://www.notion.com/help/math-equations#:~:text=Notion%20uses%20the%C2%A0KaTeX%C2%A0library%C2%A0to%20render%20math,supports%20a%20large%20subset%20of%C2%A0LaTeX%C2%A0functions) [\[20\]](https://www.notion.com/help/math-equations#:~:text=Notion%20uses%20the%C2%A0KaTeX%C2%A0library%C2%A0to%20render%20math,supports%20a%20large%20subset%20of%C2%A0LaTeX%C2%A0functions) [\[21\]](https://www.notion.com/help/math-equations#:~:text=Just%20like%20you%20can%20format,equation%2C%20like%20this%20quadratic%20formula) Math equations – Notion Help Center

<https://www.notion.com/help/math-equations>

[\[22\]](https://zhuanlan.zhihu.com/p/19777422849#:~:text=%E7%89%B9%E6%80%A7%2F%E8%BD%AF%E4%BB%B6%20Notion%20Wolai%20Obsidian%20%E5%85%AC%E5%BC%8F%E6%94%AF%E6%8C%81,%E9%9C%80%E8%A6%81%E8%87%AA%E5%B7%B1%E9%85%8D%E7%BD%AE%20%E5%9B%BE%E7%89%87%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E7%9B%B4%E6%8E%A5%E4%B8%8A%E4%BC%A0%20%E9%9C%80%E8%A6%81%E5%9B%BE%E5%BA%8A) 笔记软件的选择 - 知乎

<https://zhuanlan.zhihu.com/p/19777422849>

[\[23\]](https://www.notion.vip/insights/link-to-a-specific-block-within-a-notion-page#:~:text=Link%20to%20a%20Specific%20Block,Within%20a%20Notion%20Page) Notion VIP: Link to a Specific Block Within a Notion Page

<https://www.notion.vip/insights/link-to-a-specific-block-within-a-notion-page>
