# BÁO CÁO TIẾN ĐỘ TRIỂN KHAI NỀN TẢNG

**Dự án:** Kubernetes Platform Foundation  
**Kỳ báo cáo:** Đến ngày 21/07/2026  
**Phiên bản tài liệu:** 1.0  
**Phạm vi:** Hạ tầng Kubernetes, Platform Console, quản trị tài nguyên, MinIO và sao lưu/khôi phục

---

## 1. Tóm tắt điều hành

Dự án đã hoàn thành phần nền tảng cốt lõi để quản lý dự án và môi trường ứng dụng trên Kubernetes, bao gồm quy trình triển khai, phân quyền, giới hạn tài nguyên, quản lý addon và giám sát. Các cơ chế bảo vệ MinIO đã được mở rộng đáng kể với quota cứng, tách khóa quản trị/khóa ứng dụng, nhiều bucket độc lập và tích hợp sao lưu theo từng dự án.

Tính năng kiểm duyệt tệp bằng vùng cách ly và ClamAV đã hoàn thành phần thiết kế và triển khai chính, hiện đang ở giai đoạn kiểm thử E2E và hiệu chỉnh tài nguyên. Chính sách chặn theo MIME/loại tệp chưa hoàn tất và là hạng mục kế tiếp.

Khung sao lưu ngoài hệ thống đã sẵn sàng về mặt kỹ thuật. Việc kích hoạt vận hành định kỳ trên production phụ thuộc vào lựa chọn kho lưu trữ ngoài, thông tin truy cập chính thức, chính sách lưu giữ và buổi diễn tập khôi phục được nghiệm thu.

### Đánh giá tổng quan

| Nhóm hạng mục | Trạng thái | Nhận định |
|---|---|---|
| Nền tảng Kubernetes và quy trình triển khai | Đã triển khai | Đáp ứng vận hành môi trường dev/staging; cần tiếp tục hardening trước production |
| Platform Console | Đã triển khai | Quản lý dự án, môi trường, pipeline, addon và tài nguyên tập trung |
| Quota và cô lập tài nguyên | Đã triển khai | Có giới hạn nhiều lớp ở Platform, Project và Kubernetes namespace |
| MinIO cơ bản và multi-bucket | Đã triển khai | Quota, IAM, monitoring, nhiều bucket và backup theo bucket đã có |
| Sao lưu ngoài hệ thống | Beta/Chờ kích hoạt | Luồng kỹ thuật đã có; cần storage chính thức và diễn tập restore đầy đủ |
| Quarantine và antivirus | Đang kiểm thử | Luồng chính đã triển khai; đang xác nhận clean/infected và tài nguyên scanner |
| Chính sách MIME/loại tệp | Chưa triển khai | Dự kiến thực hiện sau khi luồng antivirus ổn định |

> **Lưu ý trạng thái:** “Đã triển khai” trong báo cáo này nghĩa là hạng mục đã có trong mã nguồn và đã được kiểm tra ở phạm vi phát triển/staging phù hợp. Một số thay đổi mới vẫn đang chờ đóng gói thành commit/release được kiểm soát. Trạng thái này không thay thế biên bản nghiệm thu production, kiểm thử tải, kiểm thử an toàn thông tin hoặc diễn tập khôi phục thảm họa.

## 2. Các hạng mục đã triển khai

### 2.1. Nền tảng Kubernetes và vận hành ứng dụng

- Xây dựng nền tảng theo hướng Kubernetes chuẩn, hỗ trợ quản trị tập trung qua Rancher.
- Tích hợp quy trình CI/CD và GitOps: mã nguồn → pipeline → image registry → cập nhật cấu hình → triển khai Kubernetes.
- Quản lý tách biệt môi trường `dev` và `prod` theo namespace.
- Tích hợp ingress, chứng chỉ TLS, theo dõi trạng thái workload và truy cập log/metric phục vụ vận hành.
- Điều chỉnh chiến lược rolling update để không xung đột với ResourceQuota khi namespace đã sử dụng gần hết hạn mức.
- Chuẩn hóa địa chỉ kết nối Rancher nội bộ để tránh lỗi DNS/TLS tái diễn sau bootstrap.

**Giá trị mang lại:** Chuẩn hóa việc triển khai ứng dụng, giảm thao tác thủ công và tạo nền tảng cho phân quyền, giám sát và kiểm soát tài nguyên tập trung.

### 2.2. Platform Console

- Quản lý dự án, môi trường, thành viên và quyền truy cập trên một giao diện tập trung.
- Hiển thị trạng thái triển khai, tài nguyên Kubernetes và thông tin pipeline.
- Tổ chức khu vực cấu hình pipeline thành các phần có thể thu gọn/mở rộng, giúp thao tác rõ ràng hơn.
- Tích hợp quản lý addon theo dự án, hiện tập trung vào Redis và MinIO.
- Bổ sung trang quản trị sao lưu và tab Backup & Recovery trong từng dự án.

**Giá trị mang lại:** Giảm phụ thuộc vào thao tác trực tiếp trên cluster và giúp đội phát triển tự phục vụ trong phạm vi được quản trị viên cho phép.

### 2.3. Quản trị và cô lập tài nguyên nhiều lớp

Hệ thống đã áp dụng mô hình kiểm soát tài nguyên theo ba lớp:

1. **Năng lực cluster/node:** Giới hạn vật lý của hạ tầng.
2. **Quota theo dự án và môi trường:** Giới hạn CPU, RAM, storage, số pod và số PVC cho từng namespace `dev`/`prod`.
3. **Policy theo addon:** Đặt mức trần cho từng loại addon và thông số cấu hình được phép.

Các thành phần đã triển khai:

- Lưu quota riêng cho từng dự án và từng môi trường.
- Đồng bộ quota xuống Kubernetes `ResourceQuota` để thực thi ở cấp cluster.
- Áp dụng `LimitRange` để pod không khai báo tài nguyên vẫn nhận request/limit mặc định.
- Kiểm tra quota ngay tại API trước khi tạo/cập nhật addon, cung cấp lỗi dễ hiểu hơn so với chờ Kubernetes từ chối.
- Giao diện quản trị cho phép admin xem và điều chỉnh quota theo dự án.
- Ghi nhận audit cho thay đổi nhạy cảm.

**Giá trị mang lại:** Hạn chế một dự án sử dụng vượt mức và ảnh hưởng tới dự án khác; đồng thời vẫn cho phép đội phát triển phân bổ tài nguyên linh hoạt trong hạn mức được cấp.

### 2.4. MinIO – lưu trữ đối tượng theo dự án

#### Chức năng nền tảng

- Cấp phát MinIO độc lập theo dự án và môi trường.
- Cung cấp thông tin kết nối S3-compatible để ứng dụng sử dụng theo chuẩn phổ biến.
- Duy trì dữ liệu khi workload khởi động lại hoặc được triển khai lại thông qua persistent volume.
- Hỗ trợ quản lý và thao tác tệp từ Platform Console trong phạm vi được phân quyền.

#### Quota và kiểm soát dung lượng

- Giới hạn dung lượng MinIO theo policy của Platform và quota của dự án.
- Thiết lập quota cứng ở cấp bucket bằng cơ chế native của MinIO.
- Kiểm tra lại quota sau khi khởi tạo và lưu trạng thái xác minh để hiển thị trên Console.
- Giới hạn kích thước một object khi upload qua Console.
- Cấp biến cấu hình `S3_MAX_OBJECT_MB` cho ứng dụng để tích hợp kiểm tra phía ứng dụng.
- Không cho giảm dung lượng instance xuống thấp hơn tổng quota đã phân bổ cho các bucket.

#### IAM và bảo vệ thông tin truy cập

- Tách khóa root quản trị MinIO khỏi khóa ứng dụng.
- Ứng dụng sử dụng service account với quyền giới hạn theo bucket, không sử dụng root credential.
- Mỗi bucket bổ sung có khóa truy cập và Kubernetes Secret riêng.
- Bucket giữa các dự án và môi trường được cô lập bằng namespace, credential và policy riêng.
- Áp dụng NetworkPolicy để giới hạn nguồn workload được phép truy cập MinIO.

#### Multi-bucket

- Hỗ trợ nhiều bucket trong cùng một MinIO instance.
- Cho phép tạo bucket, xem trạng thái và điều chỉnh quota từ Console.
- Kiểm soát tổng quota các bucket không vượt dung lượng instance.
- Duy trì bucket mặc định `app` để tương thích với ứng dụng hiện hữu.
- Phân quyền IAM độc lập cho từng bucket, hạn chế truy cập chéo.
- Tích hợp backup/restore theo từng bucket thay vì gộp toàn bộ instance.

#### Monitoring

- Hiển thị tổng dung lượng, đã sử dụng, còn lại và tỷ lệ sử dụng.
- Theo dõi trạng thái MinIO, PVC và các chỉ số request S3 có sẵn từ Prometheus.
- Hiển thị trạng thái xác minh quota native để hỗ trợ phát hiện cấu hình không đồng bộ.

**Giá trị mang lại:** MinIO đã chuyển từ mô hình “chỉ cung cấp kết nối” sang mô hình có quota, IAM, cô lập, giám sát và quản trị vòng đời rõ ràng hơn.

### 2.5. Sao lưu và khôi phục

#### Khung sao lưu cấp nền tảng

- Quản lý backup target tương thích S3 như Backblaze B2, Cloudflare R2, AWS S3 hoặc S3 tự vận hành.
- Thông tin xác thực được lưu trong Kubernetes Secret; cơ sở dữ liệu chỉ lưu tham chiếu.
- Cờ cấu hình mã hóa đã có trong metadata nhưng mã hóa client-side chưa được nghiệm thu; hiện vẫn phụ thuộc TLS và cơ chế mã hóa của nhà cung cấp storage.
- Cho phép kiểm tra kết nối backup target trước khi sử dụng.
- Hỗ trợ chạy thủ công và lập lịch tự động theo cron.
- Cho phép cấu hình giờ chạy thân thiện theo UTC+7 trên giao diện.
- Chính sách retention theo số bản backup thành công; khi có bản mới vượt số lượng giữ lại, bản thành công cũ nhất được dọn dẹp.
- Có lịch sử run và trạng thái thành công/thất bại để theo dõi vận hành.
- Dùng khóa tiến trình để tránh nhiều worker backup chạy chồng nhau.

#### Phạm vi dữ liệu backup

- PostgreSQL metadata của Platform bằng định dạng dump chuyên dụng.
- Snapshot etcd và cấu hình hạ tầng cần thiết cho quy trình DR.
- Dữ liệu MinIO theo từng dự án, môi trường và bucket.
- Manifest phiên bản hạ tầng và checksum để kiểm tra tính toàn vẹn.

#### Khôi phục

- Có script tải bản backup từ offsite storage và xác minh checksum.
- Có worker phục hồi dữ liệu MinIO theo từng dự án/bucket vào vùng tạm `__restore/...`, tránh ghi đè trực tiếp dữ liệu đang chạy.
- Admin có thể tạo yêu cầu restore từ tab Backup & Recovery của dự án.
- Metadata artifact gắn với project, environment và bucket, phục vụ khôi phục có phạm vi.

#### Trạng thái kiểm chứng

- Luồng tạo và tải backup lên S3-compatible storage đã được kiểm tra E2E trên môi trường thử nghiệm.
- Các lỗi thực tế về phân tách trường SQL, NetworkPolicy và đường dẫn checksum đã được phát hiện và khắc phục trong quá trình kiểm thử.
- Một lượt tải xuống/restore từ Backblaze B2 bị chặn bởi giới hạn giao dịch/băng thông của tài khoản thử nghiệm; đây là giới hạn nhà cung cấp, không phải kết luận nghiệm thu restore production.
- Việc kích hoạt lịch production đang được giữ ở trạng thái chờ cho đến khi có kho lưu trữ chính thức và chính sách vận hành được phê duyệt.

**Giá trị mang lại:** Có nền móng sao lưu ngoài hệ thống và khôi phục theo phạm vi dự án; giảm rủi ro phụ thuộc duy nhất vào ổ đĩa/VPS đang vận hành.

## 3. Hạng mục đang thực hiện

### 3.1. Quarantine và antivirus cho bucket nhạy cảm

Tính năng được thiết kế dạng **tùy chọn theo từng bucket**, phù hợp với dữ liệu cần kiểm soát cao như hồ sơ KYC, giấy tờ cá nhân hoặc tệp do người dùng bên ngoài tải lên. Bucket thông thường không bắt buộc bật để tránh tăng độ trễ và chi phí tài nguyên.

Luồng xử lý đã triển khai:

1. Tệp được upload vào bucket cách ly bằng credential dành riêng cho uploader.
2. API trả trạng thái tiếp nhận và tạo bản ghi scan bất đồng bộ.
3. Worker dùng ClamAV quét tệp trong Kubernetes Job cô lập.
4. Tệp sạch được chuyển sang bucket chính.
5. Tệp nhiễm mã độc được chuyển sang bucket infected và lưu tên nhận diện.
6. Khóa ứng dụng trên bucket sạch được chuyển sang chỉ đọc khi bật chế độ scan, nhằm ngăn đường upload trực tiếp bỏ qua kiểm duyệt.

Phần đã hoàn thành:

- Schema quản lý scan profile và lịch sử scan object.
- API bật scan và truy vấn trạng thái scan.
- Tạo quarantine/infected bucket và IAM riêng cho uploader/scanner.
- Worker ClamAV với image được cố định phiên bản.
- Trạng thái `queued`, `scanning`, `clean`, `infected`, `failed`.
- Giao diện hiển thị chế độ scan và thao tác bật scan.
- Hạn mức CPU/RAM cho scanner để tránh sử dụng tài nguyên không kiểm soát.

Phần đang kiểm thử:

- Xác nhận E2E tệp sạch được promote đúng bucket.
- Xác nhận EICAR được nhận diện và chuyển sang vùng infected.
- Xác nhận app key thực sự chỉ đọc sau khi bật scan.
- Hiệu chỉnh request/limit tài nguyên ClamAV; thực nghiệm cho thấy scanner cần khoảng 1 GiB RAM để hoạt động ổn định.
- Kiểm tra retry, timeout và xử lý job lỗi/OOM.
- Xác minh quy trình cập nhật chữ ký ClamAV, ghi nhận phiên bản signature và khả năng xử lý backlog.
- Pin toàn bộ image phụ trợ thay vì dùng tag động trước khi phát hành production.

**Trạng thái:** Chưa đề nghị bật mặc định trên production cho đến khi hoàn tất toàn bộ tiêu chí nghiệm thu nêu tại mục 6.

## 4. Hạng mục dự kiến tiếp theo

### Ưu tiên 1 – Hoàn tất antivirus và chính sách nội dung

- Hoàn tất bộ kiểm thử clean/infected/failed/retry cho ClamAV.
- Bổ sung chính sách allow/deny theo MIME, phần mở rộng và nhóm tệp cho từng bucket có kiểm duyệt.
- Không chỉ tin `Content-Type` do client gửi; xác định loại tệp từ nội dung/magic bytes khi phù hợp.
- Giới hạn kích thước, thời gian quét và số lần thử lại.
- Bổ sung cơ chế dọn tệp quarantine quá hạn.
- Bổ sung cảnh báo khi scan lỗi liên tục hoặc phát hiện malware.

### Ưu tiên 2 – Nghiệm thu backup/restore

- Chọn kho lưu trữ offsite chính thức và tạo credential production theo nguyên tắc quyền tối thiểu.
- Chốt lịch backup, retention và ngân sách dung lượng.
- Bật versioning/Object Lock nếu nhà cung cấp hỗ trợ và chính sách yêu cầu.
- Chạy restore drill đầy đủ cho PostgreSQL, cấu hình/etcd và MinIO project bucket.
- Đo RPO/RTO thực tế và lập biên bản kết quả.
- Xây dựng runbook khôi phục VPS/cluster mới với phiên bản hạ tầng được cố định.
- Thiết lập cảnh báo backup thất bại và cảnh báo không có bản backup mới đúng hạn.

### Ưu tiên 3 – Hardening trước production

- Kiểm thử tải cho API, MinIO và scan worker.
- Rà soát RBAC, NetworkPolicy, Secret rotation và quyền backup target.
- Kiểm thử an toàn thông tin theo phạm vi được phê duyệt.
- Chuẩn hóa dashboard/SLA cảnh báo cho dung lượng, quota, pod, PVC và backup.
- Chốt quy trình nâng cấp, rollback và bảo trì định kỳ.
- Hoàn thiện tài liệu vận hành, bàn giao và đào tạo quản trị viên.

## 5. Lộ trình đề xuất 30–60–90 ngày

### 0–30 ngày

- Kết thúc kiểm thử E2E quarantine/antivirus.
- Hoàn thiện MIME/file-type policy.
- Chốt nhà cung cấp backup và policy retention.
- Chạy restore drill đầu tiên trên môi trường tách biệt.
- Hoàn thiện checklist hardening bắt buộc trước production.

### 31–60 ngày

- Kiểm thử tải và tối ưu tài nguyên theo số liệu thực tế.
- Hoàn thiện cảnh báo tập trung cho platform, addon và backup.
- Rà soát bảo mật IAM/RBAC/NetworkPolicy và vòng đời Secret.
- Chuẩn hóa runbook sự cố, backup và khôi phục.

### 61–90 ngày

- Diễn tập DR có đo RPO/RTO.
- Hoàn thiện tài liệu bàn giao và đào tạo vận hành.
- Nghiệm thu production theo checklist chức năng, bảo mật và khôi phục.
- Lập kế hoạch mở rộng các addon cơ sở dữ liệu và chính sách backup tương ứng.

> Lộ trình trên là đề xuất và sẽ được điều chỉnh theo năng lực hạ tầng, mức độ ưu tiên nghiệp vụ, ngân sách lưu trữ và tiêu chí nghiệm thu của khách hàng.

## 6. Tiêu chí nghiệm thu đề xuất cho giai đoạn kế tiếp

### Antivirus/Quarantine

- Tệp sạch được đưa vào bucket đích và có thể đọc bằng app key.
- EICAR bị phát hiện, không xuất hiện trong bucket sạch và được lưu tại vùng infected.
- App key không thể ghi trực tiếp vào bucket sạch khi scan bắt buộc.
- Scan lỗi có retry hữu hạn, ghi rõ nguyên nhân và phát cảnh báo.
- Worker không vượt quota dự án và không ảnh hưởng đáng kể đến workload chính.

### Backup/Restore

- Backup tự động chạy đúng lịch và chỉ giữ đúng số bản đã cấu hình.
- Manifest và checksum xác minh thành công.
- Khôi phục PostgreSQL vào database tạm thành công.
- Khôi phục một bucket dự án vào vùng sandbox thành công và đối soát object/checksum đạt yêu cầu.
- Diễn tập khôi phục cluster/VPS mới có runbook, thời gian thực tế và người chịu trách nhiệm.
- Credential backup không có quyền rộng hơn nhu cầu vận hành.

### Production readiness

- Không còn lỗi mức nghiêm trọng/cao chưa có phương án xử lý.
- Dashboard và cảnh báo vận hành đã được cấu hình.
- Có quy trình rollback, xử lý sự cố và escalation.
- Tài liệu quản trị, tài liệu người dùng và biên bản bàn giao được phê duyệt.

## 7. Rủi ro, giới hạn và phụ thuộc

| Nội dung | Ảnh hưởng | Khuyến nghị |
|---|---|---|
| Chưa có backup storage production | Chưa thể đảm bảo bản sao ngoài hệ thống theo lịch | Chọn S3-compatible storage hoặc backup VPS độc lập và cấp credential chính thức |
| Restore offsite chưa được nghiệm thu trọn vẹn | Chưa có số liệu RTO/RPO tin cậy | Chạy restore drill sau khi có target không bị giới hạn trial |
| ClamAV cần khoảng 1 GiB RAM/job | Có thể cạnh tranh tài nguyên với workload dự án | Dành quota riêng, giới hạn concurrency và cân nhắc node/namespace chuyên dụng khi quy mô tăng |
| Quota bucket MinIO được cập nhật theo cơ chế scanner của MinIO | Có thể không phản ánh tức thời ở mọi thời điểm | Duy trì ngưỡng an toàn và cảnh báo trước khi đạt 100% |
| Upload S3 trực tiếp không tự động có MIME/antivirus nếu bucket không bật scan | Ứng dụng vẫn chịu trách nhiệm kiểm tra đầu vào ở bucket thường | Bật scan cho dữ liệu nhạy cảm; hoàn thiện policy MIME và hướng dẫn tích hợp |
| Các thay đổi backup, multi-bucket và scan chưa được đóng gói thành release kiểm soát | Khó truy vết chính xác phiên bản đã triển khai | Review, commit, chạy CI và phát hành bằng immutable image tag trước nghiệm thu |
| Portal/worker chưa có bằng chứng HA và một số image còn dùng tag động | Có rủi ro gián đoạn hoặc sai lệch phiên bản | Tăng replica phù hợp, pin image digest/tag và bổ sung health/SLO evidence |
| Quick login phải được xác nhận tắt ở production | Có thể tạo bề mặt truy cập không phù hợp nếu cấu hình sai | Đặt mặc định tắt, kiểm tra cấu hình sau deploy và đưa vào checklist bảo mật |
| Backup có metadata encryption nhưng chưa chứng minh mã hóa client-side | Có thể không đáp ứng yêu cầu mã hóa ngoài cơ chế của storage | Chốt yêu cầu compliance và kiểm thử mã hóa thực tế trước production |
| Hạ tầng hiện tại còn mang tính phát triển/staging | Kết quả tải và độ sẵn sàng chưa đại diện production | Thực hiện performance test, security review và DR drill trước nghiệm thu |

## 8. Đề nghị phối hợp từ khách hàng

Để hoàn tất giai đoạn tiếp theo, đề nghị khách hàng xác nhận:

1. Danh sách bucket/dữ liệu bắt buộc bật antivirus và chính sách loại tệp tương ứng.
2. Mục tiêu RPO/RTO mong muốn cho toàn hệ thống và cho từng dự án quan trọng.
3. Nhà cung cấp backup, dung lượng dự kiến và thời gian lưu giữ.
4. Khung giờ backup/bảo trì được phép.
5. Yêu cầu Object Lock, mã hóa, vị trí lưu trữ dữ liệu và tuân thủ pháp lý nếu có.
6. Tiêu chí nghiệm thu production và đầu mối xác nhận kỹ thuật/nghiệp vụ.

## 9. Kết luận

Nền tảng đã hoàn thành phần lớn năng lực cốt lõi về triển khai ứng dụng, quản lý tài nguyên và vận hành MinIO an toàn theo dự án. Multi-bucket, IAM độc lập, quota cứng, monitoring và tích hợp backup đã hình thành một nền tảng tốt để mở rộng cho các hệ thống có dữ liệu nhạy cảm.

Trọng tâm hiện tại là hoàn tất kiểm thử antivirus/quarantine, bổ sung chính sách loại tệp và nghiệm thu quy trình backup/restore với kho lưu trữ production. Sau ba hạng mục này, dự án có thể chuyển sang giai đoạn hardening, kiểm thử tải, diễn tập DR và nghiệm thu vận hành chính thức.

---

**Phân loại tài liệu:** Báo cáo tiến độ kỹ thuật – dùng cho trao đổi dự án  
**Người lập:** Đội triển khai nền tảng  
**Ngày lập:** 21/07/2026
