# 변경 이력

한국어 · [English](CHANGELOG.en.md) · [中文(원본·상류)](CHANGELOG.zh.md)

이 문서는 ARTEX 한국어판(이 포크)이 상류 저장소에 더한 변경을 기록합니다. 형식은 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) 를 참고합니다.

상류 ARTEX 프로젝트의 버전별 릴리스 이력(0.3.x 이하)과 기여자 목록은 원본 중국어 그대로 [`CHANGELOG.zh.md`](CHANGELOG.zh.md) 에 보존했습니다. 상류 변경과 대조하기 쉽도록 `README.zh.md` 와 같은 방식으로 원문을 그대로 남깁니다. 각 변경의 자세한 내용과 근거는 저장소 커밋 이력에서 확인할 수 있습니다.

## [Unreleased] · 한국어판 변경

### 보안

- **첫 실행 관리자 계정의 원격 선점을 막았습니다.** 비밀번호가 아직 없는 인스턴스는 `POST /api/auth/init` 을 인증 없이 받아, 포트에 먼저 닿은 누구나 관리자 비밀번호를 정하고 JWT 를 받을 수 있었습니다(비공개 제보). 이제 기동 시 서버 콘솔에만 출력되는 설정 토큰(`setup_token`)이 맞아야 초기화되고, 틀린 시도는 횟수 제한에 걸립니다. 무인 배포용으로 `ARTEX_SETUP_TOKEN`·`ARTEX_ADMIN_PASSWORD` 환경 변수를 추가했고, `/setup` 화면에 토큰 입력란을 넣었습니다.
- **기본 바인딩을 127.0.0.1 로 바꿨습니다.** `artex -addr` 기본값이 `:8787` 에서 `127.0.0.1:8787` 로, `docker-compose.yml` 의 8787 게시가 `${ARTEX_BIND:-127.0.0.1}` 로 바뀝니다. 다른 기기에서 접속하던 배포는 `-addr :8787` 또는 `.env` 의 `ARTEX_BIND=0.0.0.0` 을 지정해야 합니다. 비밀번호 없이 루프백이 아닌 주소에 바인딩하면 기동 로그에 경고가 찍힙니다.
- **비밀번호를 바꾸면 기존 토큰이 모두 무효화됩니다.** JWT 서명 키를 비밀번호 해시에서 파생해, 화면 변경·`reset-password.sh`·DB 직접 수정 어느 경로든 이전 세션이 끊깁니다. 업그레이드 직후에도 한 번 다시 로그인해야 합니다. 비밀번호 변경 응답에 새 토큰이 실려 현재 브라우저 세션은 이어집니다.
- **CORS 전면 허용을 허용 목록으로 바꿨습니다.** `Access-Control-Allow-Origin: *` 대신 `ARTEX_CORS_ORIGINS` 에 적힌 출처(기본값은 `next dev` 의 `http://localhost:5173`)에만 헤더를 줍니다. 서버 측 최소 비밀번호 길이(8자) 검사와 초기화·로그인 실패 로그(클라이언트 주소 포함)도 함께 추가했습니다.

### 현지화 (i18n)

- **한국어·영어·중국어 화면 및 실행 언어 선택을 연결했습니다.** 언어 전환은 `ko → en → zh → ko` 순환과 쿠키 복원을 지원하며, 날짜·접근성 문구도 현재 언어를 사용합니다. 브라우저의 작업 생성·대화 전송 요청에서 실제 `X-Artex-Locale: zh` 헤더를 관측했습니다.
- **작업과 대화의 언어 수명을 분리했습니다.** 작업 생성 시 언어를 PostgreSQL에 저장하고 재로드·재개·자식 작업·보고·재검증에 전달합니다. 대화는 새 턴의 유효한 요청 언어를 저장하되 실행 중인 턴은 시작 시 언어를 유지합니다. 빈 언어의 기존 데이터와 헤더 없는 기존 API는 사용자 입력 언어를 따릅니다.
- **프롬프트 원문과 사용자 초안을 보존합니다.** 에이전트 편집기의 언어 전환은 미저장 초안을 유지하고, 편집하지 않은 표시 번역은 저장 시 원문을 유지합니다. 실행별 출력 언어 지시를 덧붙이되 사용자 본문·명령·증거·과거 기록은 자동 번역하지 않습니다. 모델 지시는 실제 응답 언어를 보장하지 않으며 원격 모델의 언어 품질은 검증하지 않았습니다.
- **언어 회귀 검증과 실제 서비스 반영을 완료했습니다.** 격리 PostgreSQL의 작업·대화 영속화와 HTTP→DB→공급자 입력, 역할별 실행·동시 언어 격리 회귀가 건너뛰기 없이 통과했습니다. 이전 검증에서 중국어 정적 빌드·지정 Go 테스트와 탭별 요청 언어·초안 보존 등을 확인했습니다. 2026-10-29 남은 빈 함수 린트 오류를 의도 주석으로 해결하여 해당 파일 오류 0을 확인했고 TypeScript 검사·한국어 정적 빌드·Linux arm64 `embedui` 바이너리·기존 Dockerfile 이미지 빌드가 통과했습니다. 한국어 정적 HTML 30개의 CJK 누출은 0입니다. 정상 종료와 안전 게이트·백업 검증 후 서비스만 재생성해 `localhost:8787`에 반영했으며 기존 작업 4개·대화 17개와 PostgreSQL 컨테이너·서비스 지정 구성을 보존하고 `locale` 열을 확인했습니다. 실제 로그인 화면의 KO→EN→ZH→KO 전환·중국어 제목·새로고침 쿠키 유지·화면 캡처 5장·브라우저 콘솔 및 페이지 오류 0을 확인했습니다. 이미지 소유 Python 메타데이터 2개는 기반 이미지 갱신에 따라 변경됐으며, 백업 목록·무결성 검사는 완전한 복원 검증이 아닙니다. 이번 배포에서 전체 테스트·인증정보 입력·새 작업 생성·원격 모델 호출은 수행하지 않았고 모델 응답 언어 품질은 검증하지 않았습니다.
- **웹 UI 카탈로그를 분리했습니다.** Next App Router 에 `next-intl` 을 도입하고 문자열을 `web/messages/{ko,en,zh}.json` 으로 관리합니다. 중국어 카탈로그는 상류 대조용 원문뿐 아니라 실제 화면 전환에도 사용합니다.
- **서버 API 의 사용자 노출 오류·응답을 한국어로 옮겼습니다.** 브라우저로 돌아가는 HTTP 오류·응답 문구를 한국어로 교체했습니다. 단 에이전트 두뇌의 입력으로 되먹여지는 문구는 벤치마크 드리프트를 막기 위해 원문을 유지했고, 그 판정 근거는 저장소 작업 문서에 기록했습니다.
- **문서를 한국어로 정비했습니다.** 한국어 `README.md` 를 만들고 영어 `README.en.md` 를 함께 두었으며, 원본 중국어는 `README.zh.md` 로 보존했습니다.

### 방어·탐지 자료

- **방어·탐지 가이드를 추가했습니다.** 자율 AI 공격이 기존 스캐너와 무엇이 다른가, 방어자가 관측할 수 있는 지문(IoC), 진입점과 하드닝, 탐지 규칙, 사고 대응을 정리한 한국어 가이드([`docs/defense-ko.md`](docs/defense-ko.md))와 같은 내용의 영어판([`docs/defense-en.md`](docs/defense-en.md))을 두었습니다.
- **배포용 탐지 규칙을 제공합니다.** 가이드의 지문을 바로 쓸 수 있는 규칙으로 옮겼습니다. 호스트·로그 계층은 [Sigma](https://sigmahq.io) 원자·상관 규칙([`detections/sigma/`](detections/sigma/)), 네트워크 계층은 enrich 프로브와 norma SDK WebFetch 의 User-Agent 를 겨냥한 [Suricata](https://suricata.io) 규칙([`detections/suricata/`](detections/suricata/))으로 담았습니다.
- **ATT&CK 커버리지를 가시화했습니다.** 규칙이 태깅하는 기법을 MITRE ATT&CK Navigator 레이어([`detections/attack/`](detections/attack/))로 정리했습니다.
- **기계 판독 침해지표(IoC)를 표준 형식으로 제공합니다.** ARTEX 가 내보내는 고유 지문을 한 파일로 모은 CSV([`detections/indicators/artex_indicators.csv`](detections/indicators/artex_indicators.csv))와, 같은 지표를 위협 인텔리전스 플랫폼에 바로 가져올 수 있는 MISP 이벤트([`detections/indicators/artex_indicators.misp.json`](detections/indicators/artex_indicators.misp.json))로 담았습니다. 규칙이 받쳐 주는 지표는 `to_ids` 로, 호스트 포렌식 포트는 분류용 단서로 구분해 표기합니다.
- **재현 가능한 탐지 테스트를 붙였습니다.** 규칙을 실제로 돌려 증명하는 테스트 여덟 종(Sigma 구조·컴파일 검증, Sigma 실시간 이벤트 매칭, 백엔드 이식성, SigmaHQ 관례 린트, Suricata 로드·발화, ATT&CK 레이어 정합, 지표-소스 일치, MISP 내보내기 ↔ CSV 동기화)과 이를 한 번에 돌리는 일괄 러너·pre-commit 예시를 추가하고 CI 머지 게이트로 연결했습니다. Sigma 실시간 이벤트 매칭은 규칙이 컴파일될 뿐 아니라 악성 샘플 이벤트에는 실제로 발화하고 정상 이벤트에는 침묵하는지까지 원자·상관 규칙 모두에서 확인합니다.

### 저장소 정비

- **보안·오남용 경고와 국내법 고지를 넣었습니다.** README 최상단에 사용 범위, 정보통신망법·개인정보보호법 고지, 오남용 금지 경고를 추가했습니다.
- **한국어 UI 스크린샷으로 화면 미리 보기를 교체했습니다.**
- **메인테이너 런북과 기여 가이드를 정비했습니다.** 상류 동기화·번역 드리프트를 막기 위한 런북([`MAINTAINING.md`](MAINTAINING.md))과 탐지 규칙 기여 계약([`CONTRIBUTING.md`](CONTRIBUTING.md))을 두었습니다. 런북에는 릴리스 발행 파이프라인의 빌드 전제와, 태그 없이 로컬에서 그 전제를 검증하는 절차도 함께 정리했습니다.
- **푸시·PR 머지 게이트 CI 를 추가했습니다.** 상류 저장소는 태그 릴리스에서만 CI 가 돌았지만, 이 포크는 모든 푸시와 PR 에서 Go 빌드·정적 분석(`go vet`)·단위 테스트([`ci.yml`](.github/workflows/ci.yml)), 한국어 UI 정적 빌드([`web.yml`](.github/workflows/web.yml)), 문서의 저장소 내부 링크·이미지 참조 무결성([`docs.yml`](.github/workflows/docs.yml))을 돌려, 한국어화 과정에서 생긴 회귀를 머지 전에 잡습니다. 데이터베이스가 있어야 하는 통합 테스트는 패키지마다 격리된 PostgreSQL 서비스로 함께 검증합니다. 문서 링크 검사는 외부 네트워크에 의존하지 않는 결정론적 스크립트([`scripts/check-doc-links.py`](scripts/check-doc-links.py))로 돌려, 다국어 문서가 서로를 가리키는 많은 상대 링크와 화면 미리 보기 이미지가 깨진 채 머지되는 것을 막습니다. 문서 앵커(`#헤딩`) 링크도 GitHub 과 같은 slug 규칙으로 헤딩과 대조해, 헤딩 글자가 바뀌어 조용히 끊긴 목차·상호 참조 링크를 함께 잡습니다. 탐지 규칙 스위트는 위 '방어·탐지 자료' 절에서 설명한 머지 게이트가 담당합니다.
- **외부 링크 생존을 주기적으로 점검합니다.** 방어 가이드가 가리키는 사고 신고 창구·표준 참조 같은 외부 링크는 원격 서버 상태에 의존해 flaky 하므로 머지 게이트에서 빼고, 비차단 워크플로([`external-links`](.github/workflows/external-links.yml))가 매주 월요일과 수동 실행으로 브라우저 User-Agent·GET·리다이렉트 추적 점검([`scripts/check-external-links.py`](scripts/check-external-links.py))을 돌립니다. 호스트는 살아 있는데 확인 방법만 막힌 경우(봇 차단·속도 제한)와 우리가 고칠 수 없는 상류 상속 죽은 링크(allowlist)는 실패로 치지 않아, 우리 문서가 큐레이션한 외부 링크가 새로 깨질 때만 빨갛게 드러냅니다.
- **기여·거버넌스 인프라를 갖췄습니다.** 버그·기능·번역 이슈 템플릿([`.github/ISSUE_TEMPLATE/`](.github/ISSUE_TEMPLATE/))과 풀 리퀘스트 템플릿([`PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md)), 보안 취약점 신고 정책([`SECURITY.md`](SECURITY.md)), 행동 강령([`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md))을 두어, 외부 기여자가 이슈·PR·보안 신고를 일관된 양식으로 제출하도록 했습니다.
- **해외 기여자를 위한 영어 문서 레이어를 완성했습니다.** 이 저장소는 한국어가 주 언어이지만, 한국어를 읽지 못하는 기여자·보안 연구자·방어자가 같은 정보에 도달하도록 핵심 문서의 영어판을 함께 두었습니다. 영어 `README.en.md`·방어 가이드([`docs/defense-en.md`](docs/defense-en.md))에 더해, 변경 이력([`CHANGELOG.en.md`](CHANGELOG.en.md)), 보안 신고 정책([`SECURITY.en.md`](SECURITY.en.md)), 행동 강령([`CODE_OF_CONDUCT.en.md`](CODE_OF_CONDUCT.en.md)), 기여 가이드([`CONTRIBUTING.en.md`](CONTRIBUTING.en.md)), 메인테이너 런북([`MAINTAINING.en.md`](MAINTAINING.en.md)), 트래픽 증거 설계 문서([`docs/finding-traffic-evidence-en.md`](docs/finding-traffic-evidence-en.md)), 그리고 버그·기능·번역 이슈 템플릿의 영어판을 갖췄습니다. 한국어판과 영어판은 머리말에서 서로를 가리켜, 어느 언어로 들어와도 반대쪽으로 이동할 수 있습니다. (풀 리퀘스트 템플릿은 현재 한국어판만 제공합니다.)

---

상류 ARTEX 프로젝트의 버전별 릴리스 이력과 기여자 목록은 [`CHANGELOG.zh.md`](CHANGELOG.zh.md) 에서 원문 그대로 볼 수 있습니다.
