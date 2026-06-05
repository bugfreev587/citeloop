import { ArticleDetailClient } from "./article-detail-client";

export default async function ArticleDetailPage({
  params,
}: {
  params: Promise<{ id: string; articleId: string }>;
}) {
  const { id, articleId } = await params;
  return <ArticleDetailClient projectId={id} articleId={articleId} />;
}
