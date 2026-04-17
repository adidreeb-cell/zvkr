import csv
import os
import random
import uuid

from faker import Faker
from rich.console import Console
from rich.panel import Panel
from rich.progress import (
    BarColumn,
    Progress,
    SpinnerColumn,
    TextColumn,
    TimeElapsedColumn,
)
from rich.prompt import Confirm, IntPrompt, Prompt
from rich.table import Table
from rich.text import Text

# Инициализация Faker и Console (для TUI)
fake = Faker("ru_RU")
console = Console()

# --- НАСТРОЙКИ УНИВЕРСИТЕТА ---
FACULTIES = {
    "Факультет информационных технологий": [
        "Алгоритмы и структуры данных",
        "Базы данных",
        "Разработка на Go",
        "Машинное обучение",
        "Архитектура ЭВМ",
    ],
    "Экономический факультет": [
        "Микроэкономика",
        "Макроэкономика",
        "Бухгалтерский учет",
        "Налоги",
        "Финансы",
    ],
    "Юридический факультет": [
        "Уголовное право",
        "Гражданское право",
        "Теория государства и права",
        "Криминалистика",
    ],
    "Медицинский факультет": [
        "Анатомия",
        "Фармакология",
        "Хирургия",
        "Латинский язык",
        "Биохимия",
    ],
}

GRADES = [2, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5]
HEADERS = [
    "Год отчета",
    "ID Студента",
    "ФИО студента",
    "Факультет",
    "Курс",
    "Статус",
    "Год зачисления",
    "Предмет",
    "Оценка",
    "ФИО преподавателя",
]


class UniversitySimulator:
    def __init__(
        self, start_year: int, end_year: int, students_per_year: int, num_teachers: int
    ):
        self.start_year = start_year
        self.end_year = end_year
        self.students_per_year = students_per_year

        # Статичные данные ВУЗа
        self.teachers = [fake.name() for _ in range(num_teachers)]
        self.students_db = []  # Единая база студентов (без дубликатов)

        # Статистика
        self.total_grades_generated = 0
        self.files_created = []

    def _enroll_freshmen(self, year: int):
        """Зачисляет первокурсников в текущем году"""
        for _ in range(self.students_per_year):
            student = {
                "id": str(uuid.uuid4())[:8],
                "name": fake.name(),
                "faculty": random.choice(list(FACULTIES.keys())),
                "enrollment_year": year,
                "course": 1,
                "status": "Обучается",
                "archived": False,  # Флаг, чтобы не плодить пустые копии после выпуска/отчисления
            }
            self.students_db.append(student)

    def _get_yearly_records(self, year: int):
        """Генерирует строки (оценки) для всех активных студентов за конкретный год"""
        records = []

        for student in self.students_db:
            # Пропускаем тех, кто уже давно покинул ВУЗ (чтобы не было дубликатов-призраков)
            if student.get("archived"):
                continue

            if student["status"] == "Обучается":
                num_grades = random.randint(2, 5)
                subjects = random.sample(
                    FACULTIES[student["faculty"]],
                    min(num_grades, len(FACULTIES[student["faculty"]])),
                )

                for subject in subjects:
                    records.append(
                        [
                            year,
                            student["id"],
                            student["name"],
                            student["faculty"],
                            student["course"],
                            student["status"],
                            student["enrollment_year"],
                            subject,
                            random.choice(GRADES),
                            random.choice(self.teachers),
                        ]
                    )
                    self.total_grades_generated += 1
            else:
                # В академе, отчислен или выпускник — 1 запись в реестре за год без оценок
                records.append(
                    [
                        year,
                        student["id"],
                        student["name"],
                        student["faculty"],
                        student["course"],
                        student["status"],
                        student["enrollment_year"],
                        "-",
                        "-",
                        "-",
                    ]
                )
                # Если отчислен или выпустился — архивируем, чтобы в следующем году его уже не было в списках
                if student["status"] in ["Отчислен", "Выпускник"]:
                    student["archived"] = True

        return records

    def _advance_students(self):
        """Перевод студентов на следующий курс, отчисления и академы в конце года"""
        for student in self.students_db:
            if student.get("archived"):
                continue

            if student["status"] == "Обучается":
                student["course"] += 1
                if student["course"] > 5:
                    student["status"] = "Выпускник"
                else:
                    # Вероятности: 90% переход, 5% отчисление, 5% академ
                    rand = random.random()
                    if rand > 0.95:
                        student["status"] = "Отчислен"
                    elif rand > 0.90:
                        student["status"] = "В академе"
            elif student["status"] == "В академе":
                # 50% шанс вернуться к учебе
                if random.random() > 0.5:
                    student["status"] = "Обучается"

    def run(self, output_dir: str, output_mode: str):
        os.makedirs(output_dir, exist_ok=True)

        years_total = self.end_year - self.start_year + 1

        # Если выбран режим "Один файл"
        single_file_path = os.path.join(
            output_dir, f"university_all_years_{self.start_year}_{self.end_year}.csv"
        )
        single_file = None
        single_writer = None

        if output_mode == "1":
            single_file = open(
                single_file_path, mode="w", encoding="utf-8-sig", newline=""
            )
            single_writer = csv.writer(single_file, delimiter=",")
            single_writer.writerow(HEADERS)
            self.files_created.append(single_file_path)

        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            TextColumn("[progress.percentage]{task.percentage:>3.0f}%"),
            TimeElapsedColumn(),
            console=console,
        ) as progress:
            task = progress.add_task("[cyan]Симуляция жизни ВУЗа...", total=years_total)

            for year in range(self.start_year, self.end_year + 1):
                # 1. Зачисляем новых студентов
                self._enroll_freshmen(year)

                # 2. Получаем оценки за этот год
                yearly_records = self._get_yearly_records(year)

                # 3. Сохраняем данные (в общий файл или в отдельный)
                if output_mode == "1":
                    single_writer.writerows(yearly_records)
                else:
                    yearly_filename = os.path.join(output_dir, f"grades_{year}.csv")
                    with open(
                        yearly_filename, mode="w", encoding="utf-8-sig", newline=""
                    ) as y_file:
                        y_writer = csv.writer(y_file, delimiter=",")
                        y_writer.writerow(HEADERS)
                        y_writer.writerows(yearly_records)
                    self.files_created.append(yearly_filename)

                # 4. Переводим студентов на следующий год (меняем статусы)
                self._advance_students()
                progress.advance(task)

        if single_file:
            single_file.close()


def run_tui():
    """Интерактивный консольный интерфейс"""
    console.clear()

    title = Text(
        "🎓 Генератор Данных Университета 🎓", style="bold magenta", justify="center"
    )
    console.print(Panel(title, border_style="cyan", expand=False))
    console.print(
        "[gray]Инструмент симулирует историю ВУЗа по годам. Студенты сохраняют свой ID "
        "и переходят с курса на курс без создания ошибочных дубликатов.[/gray]\n"
    )

    # Интерактивные вопросы
    start_year = IntPrompt.ask(
        "📅 Введите [bold cyan]начальный год[/bold cyan] симуляции", default=2020
    )
    end_year = IntPrompt.ask(
        "📅 Введите [bold cyan]конечный год[/bold cyan] симуляции", default=2024
    )

    if start_year > end_year:
        console.print(
            "[bold red]Ошибка: Начальный год не может быть больше конечного![/bold red]"
        )
        return

    students_per_year = IntPrompt.ask(
        "🧑‍🎓 Количество [bold green]поступающих[/bold green] (в каждый год)",
        default=150,
    )
    num_teachers = IntPrompt.ask(
        "👨‍🏫 Количество [bold yellow]преподавателей[/bold yellow] (постоянный штат)",
        default=45,
    )

    console.print("\n[bold]Выберите формат сохранения файлов:[/bold]")
    console.print(
        "  [bold cyan]1.[/bold cyan] 📄 Один большой файл за все годы (удобно для БД и аналитики)"
    )
    console.print(
        "[bold cyan]2.[/bold cyan] 📁 Раздельно по годам (каждый год в новом файле)"
    )

    output_mode = Prompt.ask("Ваш выбор", choices=["1", "2"], default="1")

    folder_name = f"dataset_{start_year}_{end_year}"

    console.print("\n")
    if Confirm.ask(
        f"🚀 Начать генерацию и сохранить в папку[bold blue]{folder_name}[/bold blue]?"
    ):
        console.print("\n")

        simulator = UniversitySimulator(
            start_year, end_year, students_per_year, num_teachers
        )
        simulator.run(folder_name, output_mode)

        # Вывод красивого отчета
        console.print("\n")
        table = Table(
            title="📊 Итоги Симуляции", show_header=True, header_style="bold magenta"
        )
        table.add_column("Метрика", style="cyan")
        table.add_column("Значение", justify="right", style="green")

        table.add_row(
            "Период симуляции",
            f"{start_year} - {end_year} ({end_year - start_year + 1} лет)",
        )
        table.add_row("Всего уникальных студентов", str(len(simulator.students_db)))
        table.add_row(
            "Всего сгенерировано оценок", str(simulator.total_grades_generated)
        )
        table.add_row("Создано файлов", str(len(simulator.files_created)))

        console.print(table)
        console.print(
            f"\n✅ [bold green]Готово![/bold green] Данные сохранены в папке [bold blue]./{folder_name}/[/bold blue]\n"
        )
    else:
        console.print("[red]Отменено пользователем.[/red]")


if __name__ == "__main__":
    try:
        run_tui()
    except KeyboardInterrupt:
        console.print("\n[bold red]Выход из программы...[/bold red]")
