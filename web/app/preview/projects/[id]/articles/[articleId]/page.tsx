import { ArticlePublishedPreviewClient } from "./article-published-preview-client";

export default async function ArticlePublishedPreviewPage({
  params,
}: {
  params: Promise<{ id: string; articleId: string }>;
}) {
  const { id, articleId } = await params;
  return <ArticlePublishedPreviewClient projectId={id} articleId={articleId} />;
}
