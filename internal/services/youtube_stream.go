package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"backend/internal/repositories"
)

type YouTubeStreamService struct {
	apiKey          string
	channelOrHandle string
	resolvedID      string // cache do channel ID resolvido a partir do handle
	matchRepo       *repositories.MatchRepository
	logger          *slog.Logger
}

func NewYouTubeStreamService(apiKey, channelOrHandle string, matchRepo *repositories.MatchRepository, logger *slog.Logger) *YouTubeStreamService {
	return &YouTubeStreamService{
		apiKey:          apiKey,
		channelOrHandle: channelOrHandle,
		matchRepo:       matchRepo,
		logger:          logger,
	}
}

type ytSearchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	} `json:"items"`
}

type ytChannelsResponse struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

type liveVideo struct {
	VideoID string
	Title   string
}

func (s *YouTubeStreamService) Sync(ctx context.Context) {
	if s.apiKey == "" || s.channelOrHandle == "" {
		s.logger.Warn("youtube stream sync ignorado: YOUTUBE_API_KEY ou YOUTUBE_CHANNEL_ID não configurados")
		return
	}

	channelID, err := s.getChannelID(ctx)
	if err != nil {
		s.logger.Error("youtube stream sync: erro ao resolver channel ID", "error", err)
		return
	}

	ongoingMatches, err := s.matchRepo.FindOngoing(ctx)
	if err != nil {
		s.logger.Error("youtube stream sync: erro ao buscar partidas em andamento", "error", err)
		return
	}
	if len(ongoingMatches) == 0 {
		return
	}

	videos, err := s.fetchLiveVideos(ctx, channelID)
	if err != nil {
		s.logger.Error("youtube stream sync: erro ao buscar streams ao vivo", "error", err)
		return
	}

	for _, match := range ongoingMatches {
		streamURL := s.resolveStreamURL(match.HomeTeam, match.AwayTeam, videos)
		if err := s.matchRepo.UpdateStreamURL(ctx, match.ID, streamURL); err != nil {
			s.logger.Error("youtube stream sync: erro ao salvar stream_url", "match_id", match.ID, "error", err)
		} else if streamURL != nil {
			s.logger.Info("youtube stream sync: stream vinculado", "match_id", match.ID, "url", *streamURL)
		}
	}
}

// getChannelID retorna o channel ID pronto para uso na API de busca.
// Se o valor configurado já for um ID (começa com "UC"), usa diretamente.
// Caso contrário, resolve o handle via API e armazena em cache.
func (s *YouTubeStreamService) getChannelID(ctx context.Context) (string, error) {
	val := strings.TrimPrefix(s.channelOrHandle, "@")
	// Channel IDs do YouTube começam com "UC"
	if strings.HasPrefix(val, "UC") {
		return val, nil
	}
	if s.resolvedID != "" {
		return s.resolvedID, nil
	}
	id, err := s.resolveHandleToID(ctx, s.channelOrHandle)
	if err != nil {
		return "", err
	}
	s.resolvedID = id
	return id, nil
}

func (s *YouTubeStreamService) resolveHandleToID(ctx context.Context, handle string) (string, error) {
	// Remove o "@" se presente; a API aceita o handle sem ele via forHandle
	h := strings.TrimPrefix(handle, "@")
	apiURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/channels?part=id&forHandle=%s&key=%s",
		url.QueryEscape(h),
		url.QueryEscape(s.apiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YouTube API retornou status %d ao resolver handle", resp.StatusCode)
	}

	var chResp ytChannelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&chResp); err != nil {
		return "", err
	}
	if len(chResp.Items) == 0 {
		return "", fmt.Errorf("handle @%s não encontrado no YouTube", h)
	}
	return chResp.Items[0].ID, nil
}

func (s *YouTubeStreamService) fetchLiveVideos(ctx context.Context, channelID string) ([]liveVideo, error) {
	apiURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/search?part=snippet&channelId=%s&eventType=live&type=video&maxResults=5&key=%s",
		url.QueryEscape(channelID),
		url.QueryEscape(s.apiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube API retornou status %d", resp.StatusCode)
	}

	var ytResp ytSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&ytResp); err != nil {
		return nil, err
	}

	var videos []liveVideo
	for _, item := range ytResp.Items {
		if item.ID.VideoID != "" {
			videos = append(videos, liveVideo{
				VideoID: item.ID.VideoID,
				Title:   item.Snippet.Title,
			})
		}
	}
	return videos, nil
}

// resolveStreamURL tenta encontrar o stream mais adequado para a partida.
// Se houver apenas 1 stream ao vivo, usa-o diretamente.
// Se houver múltiplos, tenta correspondência pelo título (nome dos times).
// Fallback: primeiro stream da lista.
func (s *YouTubeStreamService) resolveStreamURL(homeTeam, awayTeam string, videos []liveVideo) *string {
	if len(videos) == 0 {
		return nil
	}

	buildURL := func(videoID string) *string {
		u := "https://www.youtube.com/embed/" + videoID
		return &u
	}

	if len(videos) == 1 {
		return buildURL(videos[0].VideoID)
	}

	homeLower := strings.ToLower(homeTeam)
	awayLower := strings.ToLower(awayTeam)
	for _, v := range videos {
		title := strings.ToLower(v.Title)
		if strings.Contains(title, homeLower) || strings.Contains(title, awayLower) {
			return buildURL(v.VideoID)
		}
	}

	return buildURL(videos[0].VideoID)
}
